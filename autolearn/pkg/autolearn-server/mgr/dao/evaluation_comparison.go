package dao

import (
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

func InsertEvalComparison(d *DAO, evalComparison *types.EvalComparison) error {
	if _, err := d.Collections.EvalComparison().InsertOne(d.Context, evalComparison); err != nil {
		return err
	}

	return nil
}

func FindEvalComparisonByID(d *DAO, projectID string, id primitive.ObjectID) (*types.EvalComparison, error) {
	filter := bson.D{
		{Key: filedID, Value: id},
		{Key: filedProjectID, Value: projectID},
	}

	singleRes := d.Collections.EvalComparison().FindOne(d.Context, filter)
	if singleRes.Err() != nil {
		if singleRes.Err() == mongo.ErrNoDocuments {
			return nil, errors.ErrorEvalComparisonNotFound
		}
		return nil, singleRes.Err()
	}

	var result types.EvalComparison
	if err := singleRes.Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func FindEvalComparisonByIDs(d *DAO, projectID string, ids []primitive.ObjectID) (map[primitive.ObjectID]*types.EvalComparison, error) {
	filter := bson.D{
		{Key: filedID, Value: bson.M{"$in": ids}},
		{Key: filedProjectID, Value: projectID},
	}

	cur, err := d.Collections.EvalComparison().Find(d.Context, filter)
	if err != nil {
		return nil, err
	}

	res := make(map[primitive.ObjectID]*types.EvalComparison)
	for cur.Next(d.Context) {
		temp := types.EvalComparison{}
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		res[temp.ID] = &temp
	}

	return res, nil
}

func InsertEvalComparisonSnapshot(d *DAO, evalComparisonSnapshot *types.EvalComparisonSnapshot) error {
	if _, err := d.Collections.ComparisonSnapshot().InsertOne(d.Context, evalComparisonSnapshot); err != nil {
		return err
	}

	return nil
}

func GetEvalComparisonSnapshotByID(d *DAO, projectID string, comparisonSnapshotID primitive.ObjectID) (*types.EvalComparisonSnapshot, error) {
	filter := bson.D{
		{Key: filedID, Value: comparisonSnapshotID},
		{Key: filedProjectID, Value: projectID},
		{Key: filedIsDeleted, Value: false},
	}

	var comparisonSnapshot types.EvalComparisonSnapshot
	if err := d.Collections.ComparisonSnapshot().FindOne(d.Context, filter).Decode(&comparisonSnapshot); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.ErrorEvalComparisonSnapshotNotFound
		}
		return nil, err
	}

	return &comparisonSnapshot, nil
}

func DeleteComparisonSnapshotByID(d *DAO, projectID string, comparisonSnapshotID primitive.ObjectID) error {
	filter := bson.D{
		{Key: filedID, Value: comparisonSnapshotID},
		{Key: filedProjectID, Value: projectID},
		{Key: filedIsDeleted, Value: false},
	}

	if _, err := d.Collections.ComparisonSnapshot().UpdateOne(d.Context, filter, bson.M{
		"$set": bson.M{filedIsDeleted: true},
	}); err != nil {
		if err != nil {
			return err
		}
		return err
	}
	return nil
}

func ListEvalComparisonSnapshot(gc *ginlib.GinContext, d *DAO, projectID string) (int64, []*types.EvalComparisonSnapshot, error) {
	filter := bson.D{
		{Key: filedProjectID, Value: projectID},
		{Key: filedIsDeleted, Value: false},
	}

	onlyMe := gc.Query("onlyMe")
	if onlyMe != "" {
		onlyMeBool, err := strconv.ParseBool(onlyMe)
		if err != nil {
			return 0, nil, err
		}
		if onlyMeBool {
			filter = append(filter, bson.E{Key: filedCreatedByID, Value: gc.GetUserID()})
		}
	}

	// 1 获取总数
	count, err := d.Collections.ComparisonSnapshot().CountDocuments(d.Context, filter)
	if err != nil {
		return 0, nil, err
	}
	if count == 0 {
		return 0, nil, nil
	}

	// 2 排序及分页展示
	opts := options.Find()
	sort := gc.GetSort()
	if sort == nil {
		opts.Sort = bson.M{filedUpdatedAt: -1}
	} else {
		opts.Sort = bson.M{filedUpdatedAt: sort.Order}
	}
	opts.Limit = &gc.GetPagination().Limit
	opts.Skip = &gc.GetPagination().Skip

	// 3 查询
	cursor, err := d.Collections.ComparisonSnapshot().Find(d.Context, filter, opts)
	if err != nil {
		return 0, nil, err
	}
	var comparisonSnapshots []*types.EvalComparisonSnapshot
	if err := cursor.All(d.Context, &comparisonSnapshots); err != nil {
		return 0, nil, err
	}
	return count, comparisonSnapshots, nil
}

func CountComparisonSnapshot(d *DAO, projectID string) (int64, error) {
	filter := bson.D{
		{Key: filedProjectID, Value: projectID},
		{Key: filedIsDeleted, Value: false},
	}

	count, err := d.Collections.ComparisonSnapshot().CountDocuments(d.Context, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}
