package dao

import (
	"context"
	"strconv"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

func insertAutoLearn(d *DAO, autoLearn *types.AutoLearn) error {
	if _, err := d.Collections.AutoLearn().InsertOne(d.Context, autoLearn); err != nil {
		return err
	}
	return nil
}

func findAutoLearnByID(d *DAO, autoLearnID, projectID string) (*types.AutoLearn, error) {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: autoLearnID},
		{Key: filedProjectID, Value: projectID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}

	var autoLearn types.AutoLearn
	if err := d.Collections.AutoLearn().FindOne(context.Background(), filter).Decode(&autoLearn); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.ErrorAutoLearnNotFound
		}
		return nil, err
	}

	return &autoLearn, nil
}

func findLastAutoLearn(gc *ginlib.GinContext, d *DAO) (*types.AutoLearnLastRevision, error) {
	var autoLearnRevision []*types.AutoLearnLastRevision
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{filedTenantID: gc.GetAuthTenantID(), filedAutoLearnRevisions + ".autoLearnState": 3, filedIsDeleted: false}}},
		bson.D{{Key: "$addFields", Value: bson.M{filedAutoLearnRevisions + "." + filedAutoLearnName: "$" + filedAutoLearnName}}},
		bson.D{{Key: "$addFields", Value: bson.M{filedAutoLearnRevisions + "." + filedTenantID: "$" + filedTenantID}}},
		bson.D{{Key: "$addFields", Value: bson.M{filedAutoLearnRevisions + "." + filedProjectID: "$" + filedProjectID}}},
		bson.D{{Key: "$project", Value: bson.M{filedID: 0, filedAutoLearnRevisions: 1}}},
		bson.D{{Key: "$unwind", Value: "$" + filedAutoLearnRevisions}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$" + filedAutoLearnRevisions}}},
		bson.D{{Key: "$match", Value: bson.M{"autoLearnState": 3}}},
		bson.D{{Key: "$sort", Value: bson.M{filedCreatedAt: -1}}},
		bson.D{{Key: "$limit", Value: 1}},
	}
	cur, err := d.Collections.AutoLearn().Aggregate(d.Context, pipeline)
	if err != nil {
		return nil, err
	}
	if err := cur.All(d.Context, &autoLearnRevision); err != nil {
		return nil, err
	}

	if len(autoLearnRevision) > 0 {
		return autoLearnRevision[0], nil
	}

	return nil, errors.ErrorAutoLearnNotFound
}

// findAutoLearn 根据关键字、页数等条件获取任务列表 TODO 将gc改为具体条件
func findAutoLearn(gc *ginlib.GinContext, d *DAO) (int64, []*types.AutoLearn, error) {
	// 1 基础过滤 用户不需要校验
	filter := bson.M{
		filedProjectID: gc.GetAuthProjectID(),
		filedTenantID:  gc.GetAuthTenantID(),
		filedIsDeleted: false,
	}

	// 2 关键字过滤
	keywords := gc.Query("keywords")
	var exactly bool
	if val := gc.Query("exactly"); val != "" {
		valB, err := strconv.ParseBool(val)
		if err == nil {
			exactly = valB
		}
	}

	if exactly && keywords != "" {
		filter[filedAutoLearnName] = keywords
	}

	var orCond []bson.M
	if !exactly && keywords != "" {
		orCond = append(orCond, bson.M{"$or": []bson.M{
			{filedAutoLearnName: primitive.Regex{Pattern: keywords, Options: ""}},
			{filedTags: primitive.Regex{Pattern: keywords, Options: ""}},
			{filedCreatedByName: primitive.Regex{Pattern: keywords, Options: ""}},
		}})
	}

	if gc.Query("autoLearnName") != "" {
		filter[filedAutoLearnName] = primitive.Regex{Pattern: gc.Query("autoLearnName"), Options: ""}
	}
	if gc.Query("createdBy") != "" {
		filter[filedCreatedByName] = primitive.Regex{Pattern: gc.Query("createdBy"), Options: ""}
	}

	revisionFilter := bson.M{filedIsDeleted: false}
	if onlyMeBool, _ := strconv.ParseBool(gc.Query("onlyMe")); onlyMeBool {
		revisionFilter[filedCreatedByID] = gc.GetUserID()
	}
	if gc.Query("revisionName") != "" {
		revisionFilter[filedRevisionName] = primitive.Regex{Pattern: gc.Query("revisionName"), Options: ""}
	}

	taskTypes := gc.QueryArray("type")
	if len(taskTypes) == 1 {
		revisionFilter["type"] = taskTypes[0]
	}

	if len(taskTypes) > 1 {
		var cond []bson.M
		for _, tye := range taskTypes {
			cond = append(cond, bson.M{"autoLearnRevisions.type": tye})
		}

		orCond = append(orCond, bson.M{"$or": cond})
	}

	filter[filedAutoLearnRevisions] = bson.M{"$elemMatch": revisionFilter}

	if len(orCond) > 0 {
		filter["$and"] = orCond
	}

	// 3 统计符合条件的doc
	total, err := d.Collections.AutoLearn().CountDocuments(d.Context, filter)
	if err != nil {
		return 0, nil, err
	}
	if total == 0 {
		return 0, nil, nil
	}

	// 4 排序及分页展示
	opts := options.Find()
	sort := gc.GetSort()
	if sort == nil {
		opts.Sort = bson.M{filedUpdatedAt: -1}
	} else {
		opts.Sort = bson.M{filedUpdatedAt: sort.Order}
	}
	opts.Limit = &gc.GetPagination().Limit
	opts.Skip = &gc.GetPagination().Skip

	// 5 查询
	cursor, err := d.Collections.AutoLearn().Find(d.Context, filter, opts)
	if err != nil {
		return 0, nil, err
	}

	var autoLearns []*types.AutoLearn
	if err := cursor.All(context.Background(), &autoLearns); err != nil {
		return 0, nil, err
	}

	for i := range autoLearns {
		var rvs []*types.AutoLearnRevision
		for j := range autoLearns[i].AutoLearnRevisions {
			rv := autoLearns[i].AutoLearnRevisions[j]
			if !rv.IsDeleted {
				if len(taskTypes) > 0 {
					for _, taskType := range taskTypes {
						if rv.Type == aisConst.AppType(taskType) {
							rvs = append(rvs, rv)
						}
					}
				} else {
					rvs = append(rvs, autoLearns[i].AutoLearnRevisions[j])
				}
			}
		}
		autoLearns[i].AutoLearnRevisions = rvs
	}

	return total, autoLearns, nil
}

func findRevisions(gc *ginlib.GinContext, d *DAO) (int64, []*types.AutoLearnRevision, error) {
	modelRevisionID := gc.Query("modelRevisionID")

	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{
			"autoLearnRevisions.continuedMeta.modelRevisionID": modelRevisionID,
			filedProjectID: gc.GetAuthProjectID(),
			filedTenantID:  gc.GetAuthTenantID(),
			filedIsDeleted: false,
			"continued":    true,
		}}},
		bson.D{{Key: "$addFields", Value: bson.M{filedAutoLearnRevisions + "." + filedAutoLearnName: "$" + filedAutoLearnName}}},
		bson.D{{Key: "$project", Value: bson.M{filedID: 0, filedAutoLearnRevisions: 1}}},
		bson.D{{Key: "$unwind", Value: "$" + filedAutoLearnRevisions}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$" + filedAutoLearnRevisions}}},
		bson.D{{Key: "$match", Value: bson.M{filedIsDeleted: false}}},
		bson.D{{Key: "$match", Value: bson.M{"continuedMeta.modelRevisionID": modelRevisionID}}},
		bson.D{{Key: "$sort", Value: bson.M{filedCreatedAt: -1}}},
	}

	var count []*struct {
		Count int64 `bson:"count"`
	}
	cur, err := d.Collections.AutoLearn().Aggregate(context.Background(), append(pipeline, bson.D{{Key: "$count", Value: "count"}}))
	if err != nil {
		log.WithError(err).Error("failed to count revision")
		return 0, nil, err
	}

	if err := cur.All(context.TODO(), &count); err != nil {
		log.WithError(err).Error("failed to decode")
		return 0, nil, err
	}
	if len(count) == 0 {
		return 0, nil, nil
	}

	pipeline = append(pipeline,
		bson.D{{Key: "$skip", Value: gc.GetPagination().Skip}},
		bson.D{{Key: "$limit", Value: gc.GetPagination().Limit}})

	cur, err = d.Collections.AutoLearn().Aggregate(context.Background(), pipeline)
	if err != nil {
		return 0, nil, err
	}

	var revisions []*types.AutoLearnRevision
	if err := cur.All(context.Background(), &revisions); err != nil {
		return 0, nil, err
	}

	return count[0].Count, revisions, nil
}

// findAutoLearnForOption 提供自动学习选项列表
func findAutoLearnForOption(d *DAO, projectID, appType string, autolearnName string) ([]*types.AutoLearn, error) {
	filter := bson.D{
		{Key: filedProjectID, Value: projectID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}

	if appType != "" {
		filter = append(filter, bson.E{Key: "autoLearnRevisions.type", Value: appType})
	}

	if autolearnName != "" {
		filter = append(filter, bson.E{Key: "autoLearnName", Value: bson.M{"$regex": primitive.Regex{Pattern: autolearnName, Options: ""}}})
	}

	ops := options.Find().SetProjection(bson.D{
		{Key: filedAutoLearnID, Value: 1},
		{Key: filedAutoLearnName, Value: 1},
		{Key: "autoLearnRevisions.revisionID", Value: 1},
		{Key: "autoLearnRevisions.revisionName", Value: 1},
		{Key: "autoLearnRevisions.modelType", Value: 1},
		{Key: "autoLearnRevisions.type", Value: 1},
		{Key: "autoLearnRevisions.isDeleted", Value: 1},
		{Key: "autoLearnRevisions.solutionType", Value: 1},
		{Key: "autoLearnRevisions.autoLearnState", Value: 1},
	})

	ops.SetLimit(20)

	cur, err := d.Collections.AutoLearn().Find(context.Background(), filter, ops)
	if err != nil {
		return nil, err
	}

	var res []*types.AutoLearn
	if err := cur.All(context.Background(), &res); err != nil {
		return nil, err
	}

	for i := range res {
		var revisions []*types.AutoLearnRevision
		for _, rev := range res[i].AutoLearnRevisions {
			if !rev.IsDeleted && ((appType != "" && string(rev.Type) == appType) || appType == "") {
				revisions = append(revisions, rev)
			}
		}
		res[i].AutoLearnRevisions = revisions
	}

	return res, nil
}

// CountAutoLearn 统计autoLearn个数
func CountAutoLearn(d *DAO, projectID, autoLearnName string) (int64, error) {
	filter := bson.D{
		{Key: filedProjectID, Value: projectID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedAutoLearnName, Value: autoLearnName},
		{Key: filedIsDeleted, Value: false},
	}

	count, err := d.Collections.AutoLearn().CountDocuments(context.Background(), filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// DeleteAutoLearnByID 通过id删除autoLearn
func DeleteAutoLearnByID(d *DAO, projectID, autoLearnID string) error {
	if _, err := d.Collections.AutoLearn().UpdateOne(context.Background(),
		bson.D{
			{Key: filedAutoLearnID, Value: autoLearnID},
			{Key: filedProjectID, Value: projectID},
			{Key: filedTenantID, Value: d.TenantID}},
		bson.M{"$set": bson.M{filedIsDeleted: true}}); err != nil {
		return err
	}

	return nil
}

func GetRunningRevisions(d *DAO) ([]*types.AutoLearnRevision, error) {
	in := func(state types.AutoLearnState) bool {
		for _, fstate := range types.NotFinalState {
			if state == fstate {
				return true
			}
		}
		return false
	}

	opt := options.Find()
	opt.SetProjection(bson.M{
		"autoLearnID":                             1,
		"tenantID":                                1,
		"projectID":                               1,
		"autoLearnRevisions.revisionID":           1,
		"autoLearnRevisions.createMode":           1,
		"autoLearnRevisions.scheduleTimeLimit":    1,
		"autoLearnRevisions.timeLimit":            1,
		"autoLearnRevisions.autoLearnState":       1,
		"autoLearnRevisions.autoLearnStateStream": 1,
	})
	cur, err := d.Collections.AutoLearn().Find(context.Background(),
		bson.M{
			filedIsDeleted:                      false,
			"autoLearnRevisions.autoLearnState": bson.M{"$in": types.NotFinalState},
		}, opt)
	if err != nil {
		return nil, err
	}

	var autoLearns []*types.AutoLearn
	if err := cur.All(context.Background(), &autoLearns); err != nil {
		return nil, err
	}

	var rvs []*types.AutoLearnRevision

	for _, autoLearn := range autoLearns {
		for _, rv := range autoLearn.AutoLearnRevisions {
			if in(rv.AutoLearnState) {
				rv.TenantID = autoLearn.TenantID
				rv.AutoLearnID = autoLearn.AutoLearnID
				rv.ProjectID = autoLearn.ProjectID
				rvs = append(rvs, rv)
			}
		}
	}

	return rvs, nil
}

// GetAlAndRvNamesByAutoLearnIDs 根据autoLearnID获取autoLearnName以及其版本的revisionName
func GetAlAndRvNamesByAutoLearnIDs(d *DAO, projectID string, autoLearnIDs []string) (map[string]string, map[string]string, error) {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: bson.M{"$in": autoLearnIDs}},
		{Key: filedProjectID, Value: projectID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}
	ops := &options.FindOptions{}
	ops.SetProjection(bson.M{
		"_id":                             0,
		filedAutoLearnName:                1,
		filedAutoLearnID:                  1,
		"autoLearnRevisions.revisionID":   1,
		"autoLearnRevisions.revisionName": 1},
	)

	var autoLearns []*types.AutoLearn
	cur, err := d.Collections.AutoLearn().Find(d.Context, filter, ops)
	if err != nil {
		return nil, nil, err
	}
	if err := cur.All(d.Context, &autoLearns); err != nil {
		return nil, nil, err
	}

	autoLearnIDToName := make(map[string]string)
	revisionIDToName := make(map[string]string)
	for _, a := range autoLearns {
		autoLearnIDToName[a.AutoLearnID] = a.AutoLearnName
		for j := range a.AutoLearnRevisions {
			if !a.AutoLearnRevisions[j].IsDeleted {
				revisionIDToName[a.AutoLearnRevisions[j].RevisionID] = a.AutoLearnRevisions[j].RevisionName
			}
		}
	}

	return autoLearnIDToName, revisionIDToName, nil
}

// GetAutoLearnByAutoLearnIDs 根据autoLearnID获取一些基本信息
func GetAutoLearnByAutoLearnIDs(d *DAO, projectID string, autoLearnIDs []string) ([]*types.AutoLearn, error) {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: bson.M{"$in": autoLearnIDs}},
		{Key: filedProjectID, Value: projectID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}
	ops := &options.FindOptions{}
	var autoLearns []*types.AutoLearn
	cur, err := d.Collections.AutoLearn().Find(d.Context, filter, ops)
	if err != nil {
		return nil, err
	}
	if err := cur.All(d.Context, &autoLearns); err != nil {
		return nil, err
	}

	return autoLearns, nil
}

func ListAutoLearnByUserID(d *DAO, userID string) ([]*types.AutoLearn, error) {
	filter := bson.D{
		{Key: filedAutoLearnRevisionsCreatedByID, Value: userID},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}
	ops := &options.FindOptions{}
	var autoLearns []*types.AutoLearn
	cur, err := d.Collections.AutoLearn().Find(d.Context, filter, ops)
	if err != nil {
		return nil, err
	}
	if err := cur.All(d.Context, &autoLearns); err != nil {
		return nil, err
	}

	return autoLearns, nil
}
