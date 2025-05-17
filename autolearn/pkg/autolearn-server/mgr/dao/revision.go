package dao

import (
	"context"
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	psClient "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg"
	publicTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types/message"
)

func insertAutoLearnRevision(d *DAO, revision *types.AutoLearnRevision, projectID string) error {
	if _, err := d.Collections.AutoLearn().UpdateOne(
		d.Context,
		bson.D{
			{Key: filedAutoLearnID, Value: revision.AutoLearnID},
			{Key: filedProjectID, Value: projectID},
			{Key: filedTenantID, Value: d.TenantID},
			{Key: filedIsDeleted, Value: false},
		},
		bson.M{
			"$push": bson.M{filedAutoLearnRevisions: revision},
			"$set":  bson.M{filedUpdatedAt: time.Now().Unix()},
		},
	); err != nil {
		return err
	}

	return nil
}

func findAutoLearnRevisionByID(d *DAO, info *BasicInfo) (*types.AutoLearnRevision, error) {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: info.AutoLearnID},
		{Key: filedProjectID, Value: info.Project},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
		{Key: filedAutoLearnRevisions, Value: bson.M{"$elemMatch": bson.M{filedRevisionID: info.RevisionID, filedIsDeleted: false}}},
	}

	type revisions struct {
		AutoLearnName      string                     `bson:"autoLearnName"`
		AutoLearnRevisions []*types.AutoLearnRevision `bson:"autoLearnRevisions"`
	}
	var rvs *revisions

	opt := options.FindOne().SetProjection(bson.M{filedID: 0, filedAutoLearnName: 1, filedAutoLearnRevisions: 1})
	if err := d.Collections.AutoLearn().FindOne(context.Background(), filter, opt).Decode(&rvs); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.ErrorAutoLearnRvNotFound
		}
		return nil, err
	}

	result := make([]*types.AutoLearnRevision, 0, 1)
	for i := range rvs.AutoLearnRevisions {
		if rvs.AutoLearnRevisions[i].RevisionID == info.RevisionID {
			rvs.AutoLearnRevisions[i].AutoLearnName = rvs.AutoLearnName
			result = append(result, rvs.AutoLearnRevisions[i])
		}
	}

	if len(result) != 1 {
		log.WithFields(log.Fields{"autoLearnID": info.AutoLearnID, "revisionID": info.RevisionID}).
			Error("multiple autoLearn revisions have the same revisionID")
		return nil, errors.ErrorDuplicateRevisionID
	}

	return result[0], nil
}

// 根据关键字、页数等条件获取任务版本列表 TODO 将gc改为具体条件
func findAutoLearnRevision(gc *ginlib.GinContext, d *DAO, autoLearnID string) (int64, bool, []*types.AutoLearnRevision, error) {
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{filedAutoLearnID: autoLearnID, filedProjectID: gc.GetAuthProjectID(), filedTenantID: gc.GetAuthTenantID(), filedIsDeleted: false}}},
		bson.D{{Key: "$project", Value: bson.M{filedID: 0, filedAutoLearnRevisions: 1}}},
		bson.D{{Key: "$unwind", Value: "$" + filedAutoLearnRevisions}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$" + filedAutoLearnRevisions}}},
		bson.D{{Key: "$match", Value: bson.M{filedIsDeleted: false}}},
		bson.D{{Key: "$sort", Value: bson.M{filedCreatedAt: -1}}},
	}

	// 0 查询是否存在不可见版本
	var hiddenNum []*struct {
		Count int64 `bson:"count"`
	}
	existInvisible := false
	curHidden, err := d.Collections.AutoLearn().Aggregate(context.Background(), append(pipeline, bson.D{{Key: "$match", Value: bson.M{filedRvIsHidden: true}}}, bson.D{{Key: "$count", Value: "count"}}))
	if err != nil {
		log.WithError(err).Error("failed to count revision")
		return 0, existInvisible, nil, err
	}
	if err = curHidden.All(context.TODO(), &hiddenNum); err != nil {
		log.WithError(err).Error("failed to decode")
		return 0, existInvisible, nil, err
	}

	if len(hiddenNum) != 0 {
		if hiddenNum[0].Count != 0 {
			existInvisible = true
		}
	}

	// 1 拼接过滤条件
	keywords := gc.Query("keywords")
	var exactly bool
	if val := gc.Query("exactly"); val != "" {
		valB, err := strconv.ParseBool(val)
		if err == nil {
			exactly = valB
		}
	}
	if exactly && keywords != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{filedRevisionName: keywords}}})
	}
	if !exactly && keywords != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{filedRevisionName: primitive.Regex{Pattern: keywords, Options: ""}}}})
	}

	if visibility := gc.Query("visibility"); visibility != "" {
		if visibility == types.VisibilityOptionVisible {
			pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{filedRvIsHidden: bson.M{"$ne": true}}}})
		}
		if visibility == types.VisibilityOptionHidden {
			pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{filedRvIsHidden: true}}})
		}
	}

	// 2 获取总数
	var count []*struct {
		Count int64 `bson:"count"`
	}
	cur, err := d.Collections.AutoLearn().Aggregate(context.Background(), append(pipeline, bson.D{{Key: "$count", Value: "count"}}))
	if err != nil {
		log.WithError(err).Error("failed to count revision")
		return 0, existInvisible, nil, err
	}
	if err := cur.All(context.TODO(), &count); err != nil {
		log.WithError(err).Error("failed to decode")
		return 0, existInvisible, nil, err
	}
	if len(count) == 0 {
		return 0, existInvisible, nil, nil
	}

	// 3 分页获取版本
	pageSize := gc.GetPagination()
	category := gc.Query("category")
	if category != "all" {
		pipeline = append(pipeline,
			bson.D{{Key: "$skip", Value: pageSize.Skip}},
			bson.D{{Key: "$limit", Value: pageSize.Limit}})
	}

	cur, err = d.Collections.AutoLearn().Aggregate(context.Background(), pipeline)
	if err != nil {
		return 0, existInvisible, nil, err
	}

	var revisions []*types.AutoLearnRevision
	if err := cur.All(context.Background(), &revisions); err != nil {
		return 0, existInvisible, nil, err
	}

	return count[0].Count, existInvisible, revisions, nil
}

// UpdateALRevisionState 更新版本状态
func UpdateALRevisionState(d *DAO, basicInfo *BasicInfo, state types.AutoLearnState, initial bool) *errors.AutoLearnError {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: basicInfo.AutoLearnID},
		{Key: filedProjectID, Value: basicInfo.Project},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}

	if initial {
		filter = append(filter, bson.E{
			Key:   filedAutoLearnRevisions,
			Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedAutoLearnState: bson.M{"$ne": types.AutoLearnStateStopped}, filedIsDeleted: false}},
		})
	} else {
		filter = append(filter, bson.E{
			Key:   filedAutoLearnRevisions,
			Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedIsDeleted: false}},
		})
	}

	res, err := d.Collections.AutoLearn().UpdateOne(d.Context, filter,
		bson.M{
			"$set": bson.M{
				"autoLearnRevisions.$.autoLearnState": state,
				"autoLearnRevisions.$.updatedAt":      time.Now().Unix(),
			},
			"$push": bson.M{
				"autoLearnRevisions.$.autoLearnStateStream": &types.AutoLearnStateStream{
					AutoLearnState: state,
					CreatedAt:      time.Now().Unix(),
				},
			},
		})
	if err != nil {
		log.WithError(err).Error("failed to update revision state")
		return errors.ErrorInternal
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	if err := sendMessage(d, basicInfo.RevisionID, state); err != nil {
		log.WithError(err).Error("failed to sendMessage")
	}

	return nil
}

func UpdateRevisionState(d *DAO, rvID string, state types.AutoLearnState, msg string) error {
	filter := bson.M{filedAutoLearnRevisionsID: rvID}

	update := bson.M{
		"$set": bson.M{
			"autoLearnRevisions.$.autoLearnState": state,
			"autoLearnRevisions.$.updatedAt":      time.Now().Unix(),
			"autoLearnRevisions.$.reason":         msg,
		},
		"$push": bson.M{
			"autoLearnRevisions.$.autoLearnStateStream": &types.AutoLearnStateStream{
				AutoLearnState: state,
				CreatedAt:      time.Now().Unix(),
			},
		},
	}

	if _, err := d.Collections.AutoLearn().UpdateOne(d.Context, filter, update); err != nil {
		return err
	}

	if err := sendMessage(d, rvID, state); err != nil {
		log.WithError(err).Error("failed to sendMessage")
	}

	return nil
}

func GetBriefRevisionByID(d *DAO, rvID string) (*types.AutoLearnRevision, error) {
	var autolearn types.AutoLearn
	if err := d.Collections.AutoLearn().FindOne(d.Context, bson.M{"autoLearnRevisions.revisionID": rvID}).Decode(&autolearn); err != nil {
		log.WithError(err).Error("")
		return nil, err
	}

	for _, rv := range autolearn.AutoLearnRevisions {
		if rv.RevisionID == rvID {
			return rv, nil
		}
	}

	return nil, fmt.Errorf("not found")
}

func GetBriefAutoLearnByRevisionID(d *DAO, rvID string) (*types.AutoLearn, error) {
	var autolearn types.AutoLearn
	if err := d.Collections.AutoLearn().FindOne(d.Context, bson.M{"autoLearnRevisions.revisionID": rvID}).Decode(&autolearn); err != nil {
		log.WithError(err).Error("")
		return nil, err
	}

	return &autolearn, nil
}

func sendMessage(d *DAO, rvID string, state types.AutoLearnState) error {
	if !types.AutoLearnStateFailedMap[state] && state != types.AutoLearnStateCompleted {
		return nil
	}
	subclass := message.SubclassAutolearnFailed
	if state == types.AutoLearnStateCompleted {
		subclass = message.SubclassAutolearnSuccessed
	}

	al, err := GetBriefAutoLearnByRevisionID(d, rvID)
	if err != nil {
		log.WithError(err).Error("failed to findAutoLearnByID")
		return err
	}

	alr, err := GetBriefRevisionByID(d, rvID)
	if err != nil {
		log.WithError(err).Error("failed to findAutoLearnRevisionByID")
		return err
	}

	if err := psClient.NewClient(config.Profile.AisEndPoint.PublicServer).CreateMessage(&psClient.MessageParam{
		UserID:   alr.CreatedBy.UserID,
		Type:     message.TypeProgress,
		Subclass: subclass,
		Addition: map[string]string{
			message.AutoLearnID:   al.AutoLearnID,
			message.AutoLearnName: al.AutoLearnName,
			message.RevisionID:    alr.RevisionID,
			message.RevisionName:  alr.RevisionName,
			message.UserName:      alr.CreatedBy.UserName,
			message.TenantID:      al.TenantID,
			message.ProjectID:     al.ProjectID,
		},
	}); err != nil {
		log.WithError(err).Error("failed to CreateMessage")
	}

	return nil
}

// AddALRevisionDataset 添加训练数据
func AddALRevisionDataset(d *DAO, basicInfo *BasicInfo, dataset *types.Dataset, initial bool) *errors.AutoLearnError {
	if dataset == nil {
		return nil
	}

	filter := bson.D{
		{Key: filedAutoLearnID, Value: basicInfo.AutoLearnID},
		{Key: filedProjectID, Value: basicInfo.Project},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
	}

	if initial {
		filter = append(filter, bson.E{
			Key:   filedAutoLearnRevisions,
			Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedAutoLearnState: bson.M{"$ne": types.AutoLearnStateStopped}, filedIsDeleted: false}},
		})
	} else {
		filter = append(filter, bson.E{
			Key:   filedAutoLearnRevisions,
			Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedIsDeleted: false}},
		})
	}

	res, err := d.Collections.AutoLearn().UpdateOne(d.Context, filter,
		bson.M{
			"$push": bson.M{
				"autoLearnRevisions.$.datasets": dataset,
			},
		})
	if err != nil {
		return errors.ErrorInternal
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

// UpdateALRevisionReason 更新版本失败的原因
func UpdateALRevisionReason(d *DAO, basicInfo *BasicInfo, reason string) error {
	updateInfo := map[string]any{filedAutoLearnRvReason: reason}
	return updateALRevisionCore(d, basicInfo, updateInfo)
}

// UpdateALRevisionName 更新版本失败的原因
func UpdateALRevisionName(d *DAO, basicInfo *BasicInfo, name string) error {
	updateInfo := map[string]any{filedRevisionName: name}
	return updateALRevisionCore(d, basicInfo, updateInfo)
}

// UpdateALRevisionVisibility 更新可见性
func UpdateALRevisionVisibility(d *DAO, basicInfo *BasicInfo, isHidden bool) error {
	updateInfo := map[string]any{filedRvIsHidden: isHidden}
	return updateALRevisionCore(d, basicInfo, updateInfo)
}

// updateALRevisionCore 版本通用更新逻辑
func updateALRevisionCore(d *DAO, basicInfo *BasicInfo, updateInfo map[string]any) error {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: basicInfo.AutoLearnID},
		{Key: filedProjectID, Value: basicInfo.Project},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
		{Key: filedAutoLearnRevisions, Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedIsDeleted: false}}},
	}

	set := bson.M{"autoLearnRevisions.$.updatedAt": time.Now().Unix()}
	for key, value := range updateInfo {
		mgoKey := fmt.Sprintf("autoLearnRevisions.$.%s", key)
		set[mgoKey] = value
	}

	res, err := d.Collections.AutoLearn().UpdateOne(d.Context, filter, bson.M{"$set": set})
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

// UpdateALRevisionDatasetVolume 更新版本中数据集的volume
func UpdateALRevisionDatasetVolume(d *DAO, basicInfo *BasicInfo, dataset *types.Dataset) error {
	filter := bson.D{
		{Key: filedAutoLearnID, Value: basicInfo.AutoLearnID},
		{Key: filedProjectID, Value: basicInfo.Project},
		{Key: filedTenantID, Value: d.TenantID},
		{Key: filedIsDeleted, Value: false},
		{Key: filedAutoLearnRevisions, Value: bson.M{"$elemMatch": bson.M{filedRevisionID: basicInfo.RevisionID, filedIsDeleted: false}}},
	}

	res, err := d.Collections.AutoLearn().UpdateOne(context.Background(), filter, bson.M{
		"$set": bson.M{
			"autoLearnRevisions.$.updatedAt":               time.Now().Unix(),
			"autoLearnRevisions.$.datasets.$[data].volume": dataset.Volume,
		},
	},
		&options.UpdateOptions{ArrayFilters: &options.ArrayFilters{
			Filters: []any{bson.M{"data.sdsPath": dataset.SDSPath}},
		}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

// DeleteALAndALRevision 删除版本以及无任何版本的实验
func DeleteALAndALRevision(gc *ginlib.GinContext, d *DAO, basicInfo *BasicInfo) error {
	if err := DeleteAutoLearnRevision(d, basicInfo); err != nil {
		return err
	}

	// if revisions is empty, autoLearn need to be delete
	total, _, _, err := ListAutoLearnRevisions(gc, d, basicInfo.AutoLearnID)
	if err != nil {
		return err
	}
	if total == 0 {
		return DeleteAutoLearnByID(d, basicInfo.Project, basicInfo.AutoLearnID)
	}

	return nil
}

func DeleteAutoLearnRevision(d *DAO, basicInfo *BasicInfo) error {
	updateInfo := map[string]any{filedIsDeleted: true}
	if err := updateALRevisionCore(d, basicInfo, updateInfo); err != nil {
		return err
	}

	return nil
}

// UpdateALRevisionEarlyStop 更新earlyStop字段
func UpdateALRevisionEarlyStop(d *DAO, basicInfo *BasicInfo, earlyStop bool) error {
	updateInfo := map[string]any{filedRevisionEarlyStop: earlyStop}
	return updateALRevisionCore(d, basicInfo, updateInfo)
}

// UpdateALRevisionEarlyStop 更新earlyStop字段
func UpdateALRevisionReLabelTaskID(d *DAO, basicInfo *BasicInfo, reLabelTaskID primitive.ObjectID) error {
	updateInfo := map[string]any{filedRvReLabelTaskID: reLabelTaskID}
	return updateALRevisionCore(d, basicInfo, updateInfo)
}

type ESRevision struct {
	ID                    primitive.ObjectID `bson:"_id" json:"datasetID"`
	CreatedAt             int64              `bson:"createdAt" json:"createdAt"`
	IsDeleted             bool               `bson:"isDeleted" json:"isDeleted"`
	AutoLearnID           string             `bson:"autoLearnID" json:"autoLearnID"`
	RevisionID            string             `bson:"revisionID" json:"revisionID"`
	RevisionState         int64              `bson:"revisionState" json:"revisionState"`
	RevisionCreatedAt     int64              `bson:"revisionCreatedAt" json:"revisionCreatedAt"`
	RevisionIsDeleted     bool               `bson:"revisionIsDeleted" json:"revisionIsDeleted"`
	RevisionModelCnt      int64              `bson:"revisionModelCnt" json:"revisionModelCnt"`
	RevisionManagedBy     string             `bson:"revisionManagedBy" json:"revisionManagedBy"`
	RevisionSnapXImageURI string             `bson:"revisionSnapXImageURI" json:"revisionSnapXImageURI"`
	RevisionType          string             `bson:"revisionType" json:"revisionType"`
	Reason                string             `bson:"reason" json:"reason"`
}

func GetAllRevisionForES(d *DAO) ([]any, error) {
	modelCnt, err := GetAllRevisionModelCnt(d)
	if err != nil {
		return nil, err
	}

	revisionModelCntMapping := make(map[string]int64)
	for index := range modelCnt {
		fmt.Printf("%s->%d", modelCnt[index].RevisionID, modelCnt[index].ModelCnt)
		revisionModelCntMapping[modelCnt[index].RevisionID] = modelCnt[index].ModelCnt
	}

	cursor, err := d.Collections.AutoLearn().Aggregate(d.Context, []bson.M{
		{"$unwind": "$autoLearnRevisions"},
		{
			"$project": bson.M{
				"_id":                   1,
				"createdAt":             1,
				"isDeleted":             "$isDeleted",
				"autoLearnID":           "$autoLearnRevisions.autoLearnID",
				"revisionID":            "$autoLearnRevisions.revisionID",
				"revisionCreatedAt":     "$autoLearnRevisions.createdAt",
				"revisionState":         "$autoLearnRevisions.autoLearnState",
				"revisionIsDeleted":     "$autoLearnRevisions.isDeleted",
				"revisionManagedBy":     "$autoLearnRevisions.managedBy",
				"revisionSnapXImageURI": "$autoLearnRevisions.snapXImageURI",
				"revisionType":          "$autoLearnRevisions.type",
				"reason":                "$autoLearnRevisions.reason",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	rvs := make([]*ESRevision, 0)
	if err := cursor.All(d.Context, &rvs); err != nil {
		return nil, err
	}

	generics := make([]any, 0)
	for idx := range rvs {
		cnt := int64(0)
		if val, exist := revisionModelCntMapping[rvs[idx].RevisionID]; exist {
			cnt = val
		}
		rvs[idx].RevisionModelCnt = cnt
		if rvs[idx].RevisionManagedBy == "" {
			rvs[idx].RevisionManagedBy = publicTypes.AISAdmissionObjectSelectorLabelValue
		}
		generics = append(generics, rvs[idx])
	}
	return generics, nil
}

type ESRevisionModelCnt struct {
	RevisionID string `bson:"_id" json:"_id"`
	ModelCnt   int64  `bson:"modelCnt" json:"modelCnt"`
}

func GetAllRevisionModelCnt(d *DAO) ([]*ESRevisionModelCnt, error) {
	cursor, err := d.Collections.AutoLearnEvaluation().Aggregate(d.Context, []bson.M{
		{
			"$unwind": "$evaluationDetails",
		},
		{
			"$match": bson.M{
				"evaluationDetails.modelFilePath": bson.M{"$ne": ""},
			},
		},
		{
			"$group": bson.M{
				"_id":    "$revisionID",
				"models": bson.M{"$addToSet": "$evaluationDetails.modelFilePath"},
			},
		},
		{
			"$project": bson.M{
				"revisionID": 1,
				"modelCnt":   bson.M{"$size": "$models"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	rvCnt := make([]*ESRevisionModelCnt, 0)
	if err := cursor.All(d.Context, &rvCnt); err != nil {
		return nil, err
	}

	return rvCnt, nil
}

func UpdateRevisionSDSDatasetClasses(d *DAO, rv *types.AutoLearnRevision) error {
	if _, err := d.Collections.AutoLearn().UpdateOne(d.Context,
		bson.M{"autoLearnRevisions.revisionID": rv.RevisionID},
		bson.M{"$set": bson.M{"autoLearnRevisions.$.datasets": bson.A{}}}); err != nil {
		return err
	}

	update := bson.M{"$set": bson.M{"autoLearnRevisions.$.datasets": rv.Datasets}}
	if _, err := d.Collections.AutoLearn().UpdateOne(d.Context,
		bson.M{"autoLearnRevisions.revisionID": rv.RevisionID},
		update); err != nil {
		return err
	}
	return nil
}
