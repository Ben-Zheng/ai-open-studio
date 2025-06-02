package site

import (
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	siteTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/site"
)

func InsertOrUpdateSite(ctx *ginlib.GinContext, payload *ReportPayload) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("sites")

	upsert := true
	opt := &options.UpdateOptions{
		Upsert: &upsert,
	}
	updater := bson.M{
		"site": payload.SiteInfo.Name,
	}
	now := time.Now().Unix()
	setter := bson.M{
		"$set": bson.M{
			"displayName": payload.SiteInfo.DisplayName,
			"adminDomain": payload.SiteInfo.AdminDomain,
			"domain":      payload.SiteInfo.Domain,
			"isCenter":    payload.SiteInfo.IsCenter,
			"isDeleted":   false,
			"lastSeen":    now,
		},
		"$setOnInsert": bson.M{
			"_id":       primitive.NewObjectID().Hex(),
			"createdAt": now,
		},
	}
	if _, err := col.UpdateOne(ctx, updater, setter, opt); err != nil {
		log.WithError(err).Errorf("failed to insert relabel task, task: %+v", payload)
		return err
	}
	return nil
}

func ListSites(ctx *ginlib.GinContext) (int64, []*siteTypes.Site, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("sites")

	filter := bson.M{
		"isDeleted": false,
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{
		{
			Key:   "isCenter",
			Value: -1,
		},
		{
			Key:   "createdAt",
			Value: -1,
		},
	})

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var rs []*siteTypes.Site
	if err := cursor.All(ctx, &rs); err != nil {
		return 0, nil, err
	}
	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}
	return total, rs, nil
}
