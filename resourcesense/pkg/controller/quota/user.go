package quota

import (
	"context"
	"time"

	"github.com/apex/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func CreateOrUpdateUserDetail(ctx *ginlib.GinContext, user *authTypes.User) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("userdetail")

	logger := log.WithFields(log.Fields{
		"userID":          user.ID,
		"userName":        user.Name,
		"userDisplayName": user.DisplayName,
	})
	now := time.Now().Unix()
	upsert := true
	opt := &options.UpdateOptions{
		Upsert: &upsert,
	}

	existTenants := make([]string, 0)
	for _, tenant := range user.Tenants {
		existTenants = append(existTenants, tenant.ID)
		isDeleted := false
		if user.Ban.Ban || user.UserStatus == string(authTypes.UserStatusDisabled) {
			isDeleted = true
		}
		if _, err := col.UpdateOne(ctx,
			bson.M{
				"userID":   user.ID,
				"tenantID": tenant.ID,
			},
			bson.M{
				"$set": bson.M{
					"userID":            user.ID,
					"userName":          user.Name,
					"userDisplayName":   user.DisplayName,
					"tenantType":        tenant.TenantType,
					"tenantID":          tenant.ID,
					"tenantName":        tenant.Name,
					"tenantDisplayName": tenant.DisplayName,
					"isDeleted":         isDeleted,
					"updatedAt":         now,
				},
				"$setOnInsert": bson.M{
					"_id":          primitive.NewObjectID(),
					"chargedQuota": quota.NilQuota().ToData().ToMongo(),
					"usedQuota":    quota.NilQuota().ToData().ToMongo(),
					"used":         quota.NilQuota().ToData().ToMongo(),
					"createdAt":    now,
				},
			}, opt); err != nil {
			logger.WithError(err).Error("update tenant detail failed")
			return err
		}
	}

	// user 从 tenant 中移除的场景
	if _, err := col.UpdateOne(ctx,
		bson.M{
			"userID": user.ID,
			"tenantID": bson.M{
				"$nin": existTenants,
			},
		},
		bson.M{
			"$set": bson.M{
				"updatedAt": now,
				"isDeleted": true,
			},
		}, nil); err != nil {
		logger.WithError(err).Error("remove old user tenant detail failed")
		return err
	}

	return nil
}

func ListUserDetails(ctx *ginlib.GinContext, tenant, userName string, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.UserDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("userdetail")
	filter := bson.M{
		"tenantID":  tenant,
		"isDeleted": false,
	}

	if userName != "" {
		filter["userName"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: userName,
				Options: "i",
			},
		}
	}

	order, by := getSorterProjection(sorter)

	pipeline := []bson.M{
		{
			"$match": filter,
		},
		{
			"$addFields": bson.M{
				"sorter": by,
			},
		},
		{
			"$sort": bson.D{
				{
					Key:   "sorter",
					Value: order,
				},
				{
					Key:   "createdAt",
					Value: 1,
				},
				{
					Key:   "userID",
					Value: 1,
				},
			},
		},
	}

	if pagination != nil {
		pipeline = append(pipeline, []bson.M{
			{
				"$skip": pagination.Skip,
			},
			{
				"$limit": pagination.Limit,
			},
		}...)
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, nil, err
	}
	var rs []*quota.UserDetail
	if err := cursor.All(ctx, &rs); err != nil {
		return 0, nil, err
	}
	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}
	for i := range rs {
		rs[i].FromDB()
	}
	return total, rs, nil
}

func GetUserDetail(ctx *ginlib.GinContext, tenant, userName string) (*quota.UserDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("userdetail")

	filter := bson.M{
		"tenantID": tenant,
		"userName": userName,
	}
	logger := log.WithFields(log.Fields{
		"tenantID": tenant,
		"userName": userName,
	})

	td := new(quota.UserDetail)
	if err := col.FindOne(context.TODO(), filter).Decode(td); err != nil {
		logger.WithError(err).Error("find user quota failed")
		return nil, err
	}
	td.FromDB()
	return td, nil
}

func GetUserDetailByID(ctx *ginlib.GinContext, tenant, userID string) (*quota.UserDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("userdetail")

	filter := bson.M{
		"tenantID": tenant,
		"userID":   userID,
	}
	logger := log.WithFields(log.Fields{
		"tenantID": tenant,
		"userID":   userID,
	})

	td := new(quota.UserDetail)
	if err := col.FindOne(ctx, filter).Decode(td); err != nil {
		logger.WithError(err).Error("find user quota failed")
		return nil, err
	}
	td.FromDB()
	return td, nil
}

func UpdateUserChargedQuota(ctx *ginlib.GinContext, tenant, userName string, chargedQuota quota.Quota) error {
	return updateUserDetail(ctx, tenant, userName, bson.M{
		"chargedQuota": chargedQuota.ToData().ToMongo(),
		"updatedAt":    time.Now().Unix(),
	})
}

func UpdateUserChargedAndUsedQuota(ctx *ginlib.GinContext, tenant, userName string, chargedQuota, usedQuota quota.Quota, used quota.Quota) error {
	return updateUserDetail(ctx, tenant, userName, bson.M{
		"chargedQuota": chargedQuota.ToData().ToMongo(),
		"usedQuota":    usedQuota.ToData().ToMongo(),
		"used":         used.ToData().ToMongo(),
	})
}

func updateUserDetail(ctx *ginlib.GinContext, tenant, userName string, updates bson.M) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("userdetail")

	filter := bson.M{
		"tenantID": tenant,
		"userName": userName,
	}
	logger := log.WithFields(log.Fields{
		"tenantID": tenant,
		"userName": userName,
		"updates":  updates,
	})

	now := time.Now().Unix()
	updates["updatedAt"] = now
	if _, err := col.UpdateOne(ctx,
		filter, bson.M{"$set": updates}); err != nil {
		logger.WithError(err).Error("update user detail failed")
		return err
	}
	return nil
}
