package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

const (
	defaultSorter       = "$createdAt"
	ColNameTenantDetail = "tenantdetail"
	ColTenantQuotaAudit = "tenant_quota_audit"
	ColGPUTypeGroup     = "gpu_type_group"
)

func Test() {
	tenant := "testqaqyzddp"
	quantity := 4
	totalQuota := quota.NilQuota()
	totalQuota.SetByResourceName(types.KubeResourceCPU, *k8sresource.NewQuantity(int64(16*1000)/1000, k8sresource.DecimalSI))
	totalQuota.SetByResourceName(types.KubeResourceMemory, *k8sresource.NewQuantity(int64(25769803776*1000)/1000, k8sresource.DecimalSI))
	totalQuota.SetByResourceName(types.KubeResourceGPU, *k8sresource.NewQuantity(int64(quantity*1000)/1000, k8sresource.DecimalSI))
	totalQuota.SetByResourceName(types.KubeResourceStorage, *k8sresource.NewQuantity(int64(107374182400*1000)/1000, k8sresource.DecimalSI))

	ctx := ginlib.NewMockGinContext()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		panic(err)
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)
	filter := bson.M{
		"tenantID": tenant,
	}

	logger := log.WithFields(log.Fields{
		"tenantID":   tenant,
		"totalQuota": totalQuota.ToData().ToMongo(),
	})

	now := time.Now().Unix()

	td := new(quota.TenantDetail)
	// 设置团队配额的时候，tenantdetail 一定是已经先同步好的
	if err := col.FindOne(context.TODO(), filter).Decode(td); err != nil {
		logger.WithError(err).Error("find tenant quota failed")
		panic(err)
	}

	dq := totalQuota.Sub(td.TotalQuota)
	// 若配额没有变化，则直接返回
	if dq.IsZero() {
		return
	}
	upsert := true
	opt := &options.UpdateOptions{
		Upsert: &upsert,
	}
	if _, err := col.UpdateOne(context.TODO(),
		bson.M{"tenantID": tenant}, bson.M{"$set": bson.M{
			"totalQuota": totalQuota.ToData().ToMongo(),
			"updatedAt":  now,
		}}, opt); err != nil {
		logger.WithError(err).Error("update tenant detail totalQuota failed")
		panic(err)
	}
}

func getSorterProjection(sorter *ginlib.Sort) (int, any) {
	if sorter == nil {
		return -1, defaultSorter
	}
	if sorter.By == "memory_percent" || sorter.By == "storage_percent" {
		ss := strings.Split(sorter.By, "_")
		if len(ss) != 2 {
			return -1, defaultSorter
		}
		return sorter.Order, bson.M{
			"$cond": bson.M{
				"if": bson.M{
					"$eq": bson.A{fmt.Sprintf("$totalQuota.%s", ss[0]), 0},
				},
				"then": bson.M{
					"$divide": bson.A{fmt.Sprintf("$chargedQuota.%s", ss[0]), 1 * 1024 * 1024 * 1024},
				},
				"else": bson.M{
					"$divide": bson.A{fmt.Sprintf("$chargedQuota.%s", ss[0]), fmt.Sprintf("$totalQuota.%s", ss[0])},
				},
			},
		}
	} else if strings.Contains(sorter.By, "_percent") {
		ss := strings.Split(sorter.By, "_")
		if len(ss) != 2 {
			return -1, defaultSorter
		}

		key := quota.EncodeMongoKeyword(ss[0])
		return sorter.Order, bson.M{
			"$cond": bson.M{
				"if": bson.M{
					"$eq": bson.A{fmt.Sprintf("$totalQuota.%s", key), 0},
				},
				"then": bson.M{
					"$divide": bson.A{fmt.Sprintf("$chargedQuota.%s", key), 1},
				},
				"else": bson.M{
					"$divide": bson.A{fmt.Sprintf("$chargedQuota.%s", key), fmt.Sprintf("$totalQuota.%s", key)},
				},
			},
		}
	} else if strings.Contains(sorter.By, "_total") {
		return sorter.Order, fmt.Sprintf("$totalQuota.%s", quota.EncodeMongoKeyword(strings.TrimSuffix(sorter.By, "_total")))
	}
	return sorter.Order, fmt.Sprintf("$chargedQuota.%s", quota.EncodeMongoKeyword(sorter.By))
}

func ListTenantDetails(ctx *ginlib.GinContext, tenantName string, pagination *ginlib.Pagination, sorter *ginlib.Sort, all bool) (int64, []*quota.TenantDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)

	filterTenantTypes := []string{}
	if !features.IsPersonalWorkspaceEnabled() {
		filterTenantTypes = append(filterTenantTypes, string(authTypes.TenantTypeWorkspace))
	}

	if consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		filterTenantTypes = append(filterTenantTypes, aisTypes.AISSystemAdminTenantName)
	}

	match := bson.M{
		"isDeleted": false,
	}
	if !all {
		match["tenantType"] = bson.M{
			"$nin": filterTenantTypes,
		}
	}

	if tenantName != "" {
		match["tenantName"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: tenantName,
				Options: "i",
			},
		}
	}

	order, by := getSorterProjection(sorter)

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$addFields": bson.M{
				"chargedQuota.apu": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$chargedQuota.nvidia@com/gpu",
								"$chargedQuota.nvidia@com/VIRTGPU",
								"$chargedQuota.huawei@com/npu",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},
				"chargedQuota.apu_ws": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$chargedQuota.nvidia@com/VIRTGPU",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},
				"chargedQuota.apu_other": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$chargedQuota.nvidia@com/gpu",
								"$chargedQuota.huawei@com/npu",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},

				"totalQuota.apu": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$totalQuota.nvidia@com/gpu",
								"$totalQuota.nvidia@com/VIRTGPU",
								"$totalQuota.huawei@com/npu",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},
				"totalQuota.apu_ws": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$totalQuota.nvidia@com/VIRTGPU",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},
				"totalQuota.apu_other": bson.M{
					"$sum": bson.M{
						"$filter": bson.M{
							"input": bson.A{
								"$totalQuota.nvidia@com/gpu",
								"$chargedQuota.huawei@com/npu",
							},
							"as":   "value",
							"cond": bson.M{"$ne": bson.A{"$$value", nil}},
						},
					},
				},
			},
		},
		{
			"$addFields": bson.M{
				"sorter": by,
				"issystemadmin": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": bson.A{"$tenantType", aisTypes.AISSystemAdminTenantName},
						},
						"then": 0,
						"else": 1,
					},
				},
			},
		},
		{
			"$sort": bson.D{
				{
					Key:   "issystemadmin",
					Value: 1,
				},
				{
					Key:   "sorter",
					Value: order,
				},
				{
					Key:   "createdAt",
					Value: 1,
				},
				{
					Key:   "tenantID",
					Value: 1,
				},
			},
		},
	}
	if pagination != nil {
		pipeline = append(pipeline, bson.M{
			"$skip": pagination.Skip,
		}, bson.M{
			"$limit": pagination.Limit,
		})
	}

	cursor, err := col.Aggregate(context.TODO(), pipeline)
	if err != nil {
		return 0, nil, err
	}
	var rs []*quota.TenantDetail
	if err := cursor.All(context.TODO(), &rs); err != nil {
		return 0, nil, err
	}
	total, err := col.CountDocuments(context.TODO(), match)
	if err != nil {
		return 0, nil, err
	}
	for i := range rs {
		rs[i].FromDB()
	}
	return total, rs, nil
}

type aggregateResource struct {
	ID       string  `bson:"_id"`
	Quantity float64 `bson:"quantity"`
}

func aggregateTenantQuotaWithField(ctx *ginlib.GinContext, col *mongo.Collection, excludeTenant, field string) (quota.QuotaData, error) {
	filter := bson.M{
		"isDeleted": false,
	}

	// 在统计某个项目的可分配配额上限时，需要排除该团队已分配配额
	if excludeTenant != "" {
		filter["tenantID"] = bson.M{
			"$ne": excludeTenant,
		}
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{
			"$project": bson.M{
				"vectors": bson.M{
					"$objectToArray": fmt.Sprintf("$%s", field),
				},
			},
		},
		{
			"$unwind": "$vectors",
		},
		{
			"$group": bson.M{
				"_id": "$vectors.k",
				"quantity": bson.M{
					"$sum": "$vectors.v",
				},
			},
		},
	}

	cursor, err := col.Aggregate(context.TODO(), pipeline)
	if err != nil {
		return nil, err
	}

	ret := quota.QuotaData{}
	var cs []*aggregateResource
	if err := cursor.All(context.TODO(), &cs); err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return ret, nil
	}

	for i := range cs {
		ret[types.KubeResourceName(cs[i].ID)] = cs[i].Quantity
	}
	return ret, nil
}

func AggregateTenantQuota(ctx *ginlib.GinContext, excludeTenant string) (*quota.TenantDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)

	totalQuota, err := aggregateTenantQuotaWithField(ctx, col, excludeTenant, "totalQuota")
	if err != nil {
		return nil, err
	}
	chargedQuota, err := aggregateTenantQuotaWithField(ctx, col, excludeTenant, "chargedQuota")
	if err != nil {
		return nil, err
	}
	usedQuota, err := aggregateTenantQuotaWithField(ctx, col, excludeTenant, "usedQuota")
	if err != nil {
		return nil, err
	}
	used, err := aggregateTenantQuotaWithField(ctx, col, excludeTenant, "used")
	if err != nil {
		return nil, err
	}
	ret := quota.TenantDetail{}
	ret.DTotalQuota = totalQuota
	ret.DChargedQuota = chargedQuota
	ret.DUsedQuota = usedQuota
	ret.DUsed = used
	ret.FromDB()
	v, _ := json.Marshal(ret)
	fmt.Println("===============================")
	fmt.Println(string(v))
	fmt.Println("===============================")
	return &ret, nil
}

func CreateOrUpdateTenantDetail(ctx *ginlib.GinContext, tenant *authTypes.Tenant) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)

	logger := log.WithFields(log.Fields{
		"tenantID":   tenant.ID,
		"tenantName": tenant.Name,
	})

	now := time.Now().Unix()
	upsert := true
	opt := &options.UpdateOptions{
		Upsert: &upsert,
	}
	if _, err := col.UpdateOne(context.TODO(),
		bson.M{"tenantID": tenant.ID},
		bson.M{
			"$set": bson.M{
				"tenantType":        tenant.TenantType,
				"tenantID":          tenant.ID,
				"tenantName":        tenant.Name,
				"tenantDisplayName": tenant.DisplayName,
				"memberCount":       tenant.UserCount,
				"updatedAt":         now,
				"isDeleted":         false,
			},
			"$setOnInsert": bson.M{
				"_id":          primitive.NewObjectID(),
				"totalQuota":   quota.NilQuota().ToData().ToMongo(),
				"chargedQuota": quota.NilQuota().ToData().ToMongo(),
				"usedQuota":    quota.NilQuota().ToData().ToMongo(),
				"used":         quota.NilQuota().ToData().ToMongo(),
				"createdAt":    now,
			},
		}, opt); err != nil {
		logger.WithError(err).Error("update tenant detail failed")
		return err
	}
	return nil
}

func ListTenantQuotaAudits(ctx *ginlib.GinContext, tenant, creator string, pagination *ginlib.Pagination) (int64, []*quota.TenantQuotaAudit, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColTenantQuotaAudit)
	filter := bson.M{
		"tenantID": tenant,
	}

	if creator != "" {
		filter["creator"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: creator,
				Options: "i",
			},
		}
	}

	findOptions := options.Find()
	findOptions.SetSkip(pagination.Skip)
	findOptions.SetLimit(pagination.Limit)
	findOptions.SetSort(bson.D{{
		Key:   "createdAt",
		Value: -1,
	}})

	cursor, err := col.Find(context.TODO(), filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var rs []*quota.TenantQuotaAudit
	if err := cursor.All(context.TODO(), &rs); err != nil {
		return 0, nil, err
	}
	total, err := col.CountDocuments(context.TODO(), filter)
	if err != nil {
		return 0, nil, err
	}
	for i := range rs {
		rs[i].FromDB()
	}
	return total, rs, nil
}

func ListAllTenantDetails() ([]*quota.TenantDetail, error) {
	ctx := ginlib.NewMockGinContext()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)
	match := bson.M{
		"isDeleted": false,
	}
	if !features.IsPersonalWorkspaceEnabled() {
		match["tenantType"] = bson.M{
			"$nin": []string{string(authTypes.TenantTypeWorkspace)},
		}
	}
	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$project": bson.M{
				"tenantID":     1,
				"totalQuota":   1,
				"chargedQuota": 1,
			},
		},
	}
	allowDiskUse := true
	cursor, err := col.Aggregate(context.TODO(), pipeline, &options.AggregateOptions{
		AllowDiskUse: &allowDiskUse,
	})
	if err != nil {
		return nil, err
	}
	var rs []*quota.TenantDetail
	if err := cursor.All(context.TODO(), &rs); err != nil {
		return nil, err
	}
	for i := range rs {
		rs[i].FromDB()
	}
	return rs, nil
}
