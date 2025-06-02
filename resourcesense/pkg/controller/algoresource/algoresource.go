package algoresource

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/algoresource"
)

func List(ctx *ginlib.GinContext, tenantName string, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*algoresource.TenantAlgo, error) {
	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	total, rs, err := authClient.ListTenant(ctx)
	if err != nil {
		return 0, nil, err
	}
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}

	colAlgo := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo")

	var metas []*algoresource.TenantAlgo
	for _, detail := range rs {
		filter := bson.D{
			{Key: "tenantID", Value: detail.ID},
			{Key: "tenantName", Value: detail.Name},
			{Key: "isDeleted", Value: false},
		}
		num, err := colAlgo.CountDocuments(ctx, filter)
		if err != nil {
			log.WithError(err).Error("failed to get CountDocuments")
			return 0, nil, err
		}

		if num == 0 {
			metas = append(metas, &algoresource.TenantAlgo{
				TenantID:    detail.ID,
				TenantName:  detail.Name,
				MemberCount: detail.UserCount,
				CreatedAt:   detail.CreatedAt,
			})
			continue
		}

		cur, err := colAlgo.Find(ctx, filter)
		if err != nil {
			log.WithError(err).Error("failed to Find")
			return 0, nil, err
		}

		var atas []*algoresource.TenantAlgo
		if err := cur.All(ctx, &atas); err != nil {
			return 0, nil, err
		}
		for _, ata := range atas {
			ata.MemberCount = detail.UserCount
			metas = append(metas, ata)
		}

		if err := cur.Close(ctx); err != nil {
			log.WithError(err).Error("failed to Close")
			return 0, nil, err
		}
	}

	return total, metas, nil
}

func AppendOrRegexFilter(filter *bson.D, fieldName string, keysword string) {
	regexFilter := bson.D{{Key: fieldName, Value: primitive.Regex{Pattern: keysword, Options: "i"}}}
	*filter = append(*filter, bson.E{Key: "$or", Value: bson.A{regexFilter}})
}

func GetByTenantID(ctx *ginlib.GinContext, tenantID string) (*algoresource.TenantAlgo, error) {
	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	t, err := authClient.GetTenant(ctx, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetTenant")
		return nil, err
	}

	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo")

	filter := bson.D{
		{Key: "tenantID", Value: tenantID},
		{Key: "isDeleted", Value: false},
	}

	var ta algoresource.TenantAlgo

	if err := col.FindOne(ctx, filter).Decode(&ta); err == mongo.ErrNoDocuments {
		ta = algoresource.TenantAlgo{
			ID:         primitive.NewObjectID(),
			TenantID:   tenantID,
			TenantName: t.Name,
			CreatedAt:  time.Now().Unix(),
			UpdatedAt:  time.Now().Unix(),
		}
		if _, err := col.InsertOne(ctx, &ta); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	ta.MemberCount = t.UserCount

	return &ta, nil
}

func Update(ctx *ginlib.GinContext, id primitive.ObjectID, algoType algoresource.AlgoType, contents []*algoresource.Content) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo")

	filter := bson.M{
		"_id":       id,
		"isDeleted": false,
	}

	set := bson.M{"updatedAt": time.Now().Unix()}
	if algoType == algoresource.AlgoCard {
		set["algoCards"] = contents
	} else {
		set["platforms"] = contents
	}

	_, err = col.UpdateOne(ctx, filter, bson.M{
		"$set": set,
	})

	return err
}

func ListAudit(ctx *ginlib.GinContext, tenantID string, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*algoresource.TenantAlgoAudit, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo_audit")

	filter := bson.D{
		{Key: "tenantID", Value: tenantID},
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		log.WithError(err).Error("failed to CountDocuments")
		return 0, nil, err
	}

	opts := options.Find()

	if sort != nil {
		opts.Sort = map[string]any{sort.By: sort.Order}
	}

	opts.Limit = &pagination.Limit
	opts.Skip = &pagination.Skip

	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		log.WithError(err).Error("failed to Find")
		return 0, nil, err
	}
	defer cur.Close(ctx)

	var metas []*algoresource.TenantAlgoAudit
	err = cur.All(ctx, &metas)
	if err != nil {
		return 0, nil, err
	}

	return total, metas, nil
}

func CreateAudit(ctx *ginlib.GinContext, taa *algoresource.TenantAlgoAudit) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo_audit")
	_, err = col.InsertOne(ctx, taa)
	return err
}

func CreateTenantProjectRecord(ctx *ginlib.GinContext, tpr *algoresource.TenantProjectAlgo) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_project_algo")
	_, err = col.InsertOne(ctx, tpr)
	return err
}

func GetTenantProjectRecordByID(ctx *ginlib.GinContext, tenantID, projectID, recordID string) (*algoresource.TenantProjectAlgo, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_project_algo")

	filter := bson.D{
		{Key: "tenantID", Value: tenantID},
		{Key: "projectID", Value: projectID},
		{Key: "recordID", Value: recordID},
	}

	var ta algoresource.TenantProjectAlgo

	if err := col.FindOne(ctx, filter).Decode(&ta); err == mongo.ErrNoDocuments {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &ta, nil
}

func ListDistributedAlgoResourceByID(ctx *ginlib.GinContext, algoType, recordID string) ([]*algoresource.TenantProjectAlgo, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_project_algo")

	filter := bson.D{
		{Key: "algoType", Value: algoType},
		{Key: "recordID", Value: recordID},
		{Key: "isDeleted", Value: false},
	}
	opts := options.Find()

	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		log.WithError(err).Error("failed to Find")
		return nil, err
	}
	defer cur.Close(ctx)

	var metas []*algoresource.TenantProjectAlgo
	err = cur.All(ctx, &metas)
	if err != nil {
		return nil, err
	}
	return metas, nil
}

func DeleteDistributedRecord(ctx *ginlib.GinContext, tenantID, projectID, algoType, recordID string) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}

	// 删除 tenant_project_algo 表中的记录
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_project_algo")
	filter := bson.D{
		{Key: "tenantID", Value: tenantID},
		{Key: "projectID", Value: projectID},
		{Key: "algoType", Value: algoType},
		{Key: "recordID", Value: recordID},
		{Key: "isDeleted", Value: false},
	}
	_, err = col.DeleteOne(context.Background(), filter)
	if err != nil {
		err := fmt.Errorf("delete codebase failed")
		log.WithField("codeID", recordID).Error(err.Error())
		return err
	}
	return nil
}

func DeleteDistributedTenantRecord(ctx *ginlib.GinContext, tenantID, algoType, recordID string) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}

	// update tenant_algo 表中的记录
	colAlgo := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo")
	filterAlgo := bson.D{
		{Key: "tenantID", Value: tenantID},
		{Key: "isDeleted", Value: false},
	}
	opts := options.Find()

	cur, err := colAlgo.Find(ctx, filterAlgo, opts)
	if err != nil {
		log.WithError(err).Error("failed to Find")
		return err
	}
	defer cur.Close(ctx)

	var meta []*algoresource.TenantAlgo
	if err := cur.All(ctx, &meta); err != nil {
		return err
	}

	var results []*algoresource.Content
	var newResults []*algoresource.Content
	if algoType == string(algoresource.Platform) {
		results = meta[0].Platforms
	} else if algoType == string(algoresource.AlgoCard) {
		results = meta[0].AlgoCards
	}

	for _, item := range results {
		if item.ID != recordID {
			newResults = append(newResults, item)
		}
	}
	if err = Update(ctx, meta[0].ID, algoresource.AlgoType(algoType), newResults); err != nil {
		log.WithError(err).Error("failed to Update")
		return err
	}

	// 增加历史记录
	taa := &algoresource.TenantAlgoAudit{
		ID:         primitive.NewObjectID(),
		TenantID:   meta[0].TenantID,
		TenantName: meta[0].TenantName,
		AlgoType:   algoresource.AlgoType(algoType),
		Records:    newResults,
		CreatorID:  ctx.GetUserID(),
		Creator:    ctx.GetUserName(),
		CreatedAt:  time.Now().Unix(),
	}
	if err := CreateAudit(ctx, taa); err != nil {
		log.WithError(err).Error("failed to CreateAudit")
		return err
	}

	return nil
}

func GetDistributedTenantRecord(ctx *ginlib.GinContext, tenantID, algoType, recordID string) ([]*algoresource.Content, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}

	colAlgo := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("tenant_algo")
	filterAlgo := bson.D{
		{Key: "tenantID", Value: tenantID},
		{Key: "isDeleted", Value: false},
	}
	opts := options.Find()

	cur, err := colAlgo.Find(ctx, filterAlgo, opts)
	if err != nil {
		log.WithError(err).Error("failed to Find")
		return nil, err
	}
	defer cur.Close(ctx)

	var meta []*algoresource.TenantAlgo
	if err := cur.All(ctx, &meta); err != nil {
		return nil, err
	}

	var results []*algoresource.Content
	var newResults []*algoresource.Content
	results = meta[0].Platforms
	if algoType == string(algoresource.AlgoCard) {
		results = meta[0].AlgoCards
	}

	for _, item := range results {
		if item.ID == recordID {
			newResults = append(newResults, item)
		}
	}

	return newResults, nil
}
