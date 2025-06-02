package resource

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

func ListAudit(ctx *ginlib.GinContext, userIDs, tenantIDs, resources, operations, datasetIDs, datasetRvIDs []string) (int64, []resource.Audit, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("audit")

	findOptions := options.Find()
	findOptions.SetSkip(ctx.GetPagination().Skip)
	findOptions.SetLimit(ctx.GetPagination().Limit)
	findOptions.SetSort(bson.M{"timestamp": -1})

	filter := bson.M{}
	if len(userIDs) != 0 {
		filter["userID"] = bson.M{"$in": userIDs}
	}
	if len(tenantIDs) != 0 {
		filter["tenantID"] = bson.M{"$in": tenantIDs}
	}
	if len(resources) != 0 {
		filter["resource"] = bson.M{"$in": resources}
	}
	if len(operations) != 0 {
		filter["operation"] = bson.M{"$in": operations}
	}
	if len(datasetIDs) != 0 {
		filter["datasets.id"] = bson.M{"$in": datasetIDs}
	}
	if len(datasetRvIDs) != 0 {
		filter["datasets.rvID"] = bson.M{"$in": datasetRvIDs}
	}

	cursor, err := col.Find(context.TODO(), filter, findOptions)
	if err != nil {
		return 0, nil, err
	}

	var as []resource.Audit
	if err := cursor.All(context.TODO(), &as); err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(context.TODO(), filter)
	if err != nil {
		return 0, nil, err
	}

	return total, as, nil
}

func CreateAudit(ctx *ginlib.GinContext, na *resource.Audit) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("audit")

	if _, err := col.InsertOne(context.TODO(), na); err != nil {
		return err
	}

	return nil
}

type TenantProject struct {
	TenantID  string `bson:"_id"`
	ProjectID string `bson:"projectID"`
}

func GetTenantIDsWithDatasetsNotEmtpy(ctx *ginlib.GinContext) ([]*TenantProject, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("audit")

	pipeline := []bson.M{
		{"$project": bson.M{"datasets": 1, "tenantID": 1, "projectID": 1}},
		{"$match": bson.M{"datasets": bson.M{"$type": "array", "$ne": bson.A{}}}},
		{"$group": bson.M{"_id": "$tenantID", "projectID": bson.M{"$addToSet": "$projectID"}}},
		{"$unwind": "$projectID"},
	}
	cursor, err := col.Aggregate(context.TODO(), pipeline)
	if err != nil {
		return nil, err
	}

	var rows []*TenantProject
	if err := cursor.All(context.TODO(), &rows); err != nil {
		return nil, err
	}

	return rows, nil
}
