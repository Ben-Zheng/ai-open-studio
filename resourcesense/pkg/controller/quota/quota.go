package quota

import (
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"strings"
	"time"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	resourceCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func GetClusterTotalQuota(gc *ginlib.GinContext) (quota.Quota, quota.Quota) {
	totalQuota, schedulableQuota := resourceCtrl.AggregateTotalResource(gc)
	if consts.ConfigMap.QuotaConfig.FixedQuota != nil {
		// 若部署配置了固定的总配额，则优先使用配置
		fixedTotalQuota := quota.BuildQuotaFromData(consts.ConfigMap.QuotaConfig.FixedQuota)
		for k, v := range fixedTotalQuota {
			if v.IsZero() {
				continue
			}
			totalQuota[k] = v
		}
	}
	// totalQuota 等价于集群物理资源总和（包含不可调度的节点）
	return totalQuota, schedulableQuota
}

func GetTenantDetail(ctx *ginlib.GinContext, tenant string) (*quota.TenantDetail, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)

	filter := bson.M{
		"tenantID": tenant,
	}
	logger := log.WithFields(log.Fields{
		"tenantID": tenant,
	})

	td := new(quota.TenantDetail)
	if err := col.FindOne(context.TODO(), filter).Decode(td); err != nil {
		logger.WithError(err).Error("find tenant quota failed")
		return nil, err
	}
	td.FromDB()

	return td, nil
}

func CheckAuth(ctx *ginlib.GinContext, tid, selfUid string) error {
	exceptHost := strings.Split(auditlib.AuditEndpoint, ":")[0]
	if strings.EqualFold(ctx.Request.Host, exceptHost) {
		return nil
	}

	client := consts.AuthMongoClient

	col := client.Database(consts.ConfigMap.AuthConf.MongoDBName).Collection("users")

	if selfUid != "" {
		if count, err := col.CountDocuments(ctx, bson.M{"id": selfUid, "tenants.id": tid}); err != nil || count == 0 {
			log.WithError(err).Error("CheckUserAndTenant selfUid check tenant failed")
			return errors.New("check tenant failed")
		}
	}

	var user types.User
	if err := col.FindOne(ctx, bson.M{"id": selfUid}).Decode(&user); err != nil {
		log.WithError(err).Errorf("find user %s failed", selfUid)
		return err
	}

	hasPermission := false
	for _, tenant := range user.Tenants {
		if tenant.ID == tid {
			if tenant.Role.Name == "role:tenant-owner" ||
				tenant.Role.Name == "role:tenant-admin" ||
				tenant.Role.Name == "role:tenant-member" {
				hasPermission = true
				break
			}
		}
	}
	if hasPermission == false {
		return errors.New("user tenant not have permission")
	}

	return nil
}

func CheckIsAdminToken(ctx *ginlib.GinContext, aisToken, selfUid string) error {
	if selfUid == "" || aisToken == "" {
		return errors.New("selfUid or aisToken is empty")
	}
	accessToken, err := types.GetToken(aisToken)
	if err != nil {
		log.WithError(err).Error("GetToken failed")
		return err
	}

	client := consts.AuthMongoClient
	if err != nil {
		log.WithError(err).Error("CheckIsAdminToken failed connect aisauth mongo")
		return err
	}
	colUser := client.Database(consts.ConfigMap.AuthConf.MongoDBName).Collection("users")
	colSession := client.Database(consts.ConfigMap.AuthConf.MongoDBName).Collection("sessions")

	var user types.User
	if err := colUser.FindOne(ctx, bson.M{"id": selfUid}).Decode(&user); err != nil {
		log.WithError(err).Errorf("find admin failed,admin: %s", selfUid)
		return err
	}
	if user.DomainType != types.AdminDomain {
		log.Errorf("users domainType is not admin: %s", selfUid)
		return errors.New("users is not admin")
	}

	var userSessionData types.Session
	if err := colSession.FindOne(ctx, bson.M{"userID": selfUid}).Decode(&userSessionData); err != nil {
		log.WithError(err).Errorf("find admin session failed,admin: %s", selfUid)
		return err
	}
	if userSessionData.AdminLoginInfo == nil || userSessionData.AdminLoginInfo.Token != accessToken {
		log.Errorf("check admin failed %s", selfUid)
		return errors.New("check admin failed")
	}

	return nil
}

func UpdateTenantQuota(ctx *ginlib.GinContext, tenant string, totalQuota quota.Quota) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}

	logger := log.WithFields(log.Fields{"tenantID": tenant})
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)
	auditCol := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColTenantQuotaAudit)
	filter := bson.M{"tenantID": tenant}
	now := time.Now().Unix()

	// 设置团队配额的时候，tenantdetail 一定是已经先同步好的
	td := new(quota.TenantDetail)
	if err = col.FindOne(context.TODO(), filter).Decode(td); err != nil {
		logger.Error("find tenant quota failed")
		return err
	}
	td.FromDB()
	dq := totalQuota.Sub(td.TotalQuota)

	// 若配额没有变化，则直接返回
	if dq.IsZero() {
		return nil
	}

	logger.WithFields(log.Fields{
		"tenantQuota": td.TotalQuota.ToData(),
		"totalQuota":  totalQuota.ToData(),
	})
	if _, err = col.UpdateOne(context.TODO(),
		bson.M{"tenantID": tenant}, bson.M{"$set": bson.M{
			"totalQuota": totalQuota.ToData().ToMongo(),
			"updatedAt":  now,
		}}, options.Update().SetUpsert(true)); err != nil {
		logger.Error("update tenant detail totalQuota failed")
		return err
	}

	diff := &quota.TenantQuotaAudit{
		ID:                primitive.NewObjectID(),
		TenantID:          tenant,
		TenantName:        td.TenantName,
		TenantDisplayName: td.TenantDisplayName,
		TotalQuota:        totalQuota,
		DiffQuota:         dq,
		CreatedAt:         now,
		Creator:           ctx.GetUserName(),
		CreatorID:         ctx.GetUserID(),
	}
	diff.ToDB()
	if _, err = auditCol.InsertOne(context.TODO(), diff); err != nil {
		logger.Error("failed to insert TenantQuotaAudit")
		return err
	}
	logger.Debugf("update tenant detail total quota success: %+v", diff)
	return nil
}

func UpdateChargedQuota(ctx *ginlib.GinContext, tenant string, chargedQuota quota.Quota) error {
	return updateTenantDetail(ctx, tenant, bson.M{
		"chargedQuota": chargedQuota.ToData().ToMongo(),
		"updatedAt":    time.Now().Unix(),
	})
}

func UpdateChargedAndUsedQuota(ctx *ginlib.GinContext, tenant string, chargedQuota, usedQuota quota.Quota, used quota.Quota) error {
	return updateTenantDetail(ctx, tenant, bson.M{
		"chargedQuota": chargedQuota.ToData().ToMongo(),
		"usedQuota":    usedQuota.ToData().ToMongo(),
		"used":         used.ToData().ToMongo(),
	})
}

func DeleteTenantDetail(ctx *ginlib.GinContext, tenant string) error {
	return updateTenantDetail(ctx, tenant, bson.M{
		"isDeleted": true,
	})
}

func updateTenantDetail(ctx *ginlib.GinContext, tenant string, updates bson.M) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail)

	filter := bson.M{
		"tenantID": tenant,
	}
	logger := log.WithFields(log.Fields{
		"tenantID": tenant,
		"updates":  updates,
	})

	now := time.Now().Unix()
	updates["updatedAt"] = now
	if _, err := col.UpdateOne(context.TODO(), filter, bson.M{"$set": updates}); err != nil {
		logger.WithError(err).Error("update tenant detail failed")
		return err
	}
	return nil
}

func UpdateTenantQuotaAndGPUGroup(gc *ginlib.GinContext, tds []*quota.TenantDetail, auditEvents []interface{}, gpuGroups []*quota.GPUGroup) error {
	c, err := consts.MongoClient.GetMongoClient(gc)
	if err != nil {
		return err
	}

	return c.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		if err := UpdateAllTenantChargedQuota(sctx, c, tds); err != nil {
			log.WithError(err).Error("caught exception during transaction in UpdateAllTenantChargedQuota, aborting...")
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in UpdateAllTenantChargedQuota")
			}
			return err
		}

		if err := InsertTenantQuotaAudit(sctx, c, auditEvents); err != nil {
			log.WithError(err).Error("caught exception during transaction in InsertTenantQuotaAudit, aborting...")
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in InsertTenantQuotaAudit")
			}
			return err
		}

		if err := UpdateOrInsertGPUGroup(sctx, c, gpuGroups); err != nil {
			log.WithError(err).Error("caught exception during transaction in UpdateOrInsertGPUGroup, aborting...")
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in UpdateOrInsertGPUGroup")
			}
			return err
		}

		err = sctx.CommitTransaction(sctx)
		switch e := err.(type) {
		case nil:
			return nil
		case mongo.CommandError:
			if e.HasErrorLabel("UnknownTransactionCommitResult") {
				log.Error("UnknownTransactionCommitResult")
			}
			return e
		default:
			return e
		}
	})
}

func UpdateAllTenantChargedQuota(ctx context.Context, c *mongo.Client, tds []*quota.TenantDetail) error {
	if len(tds) == 0 {
		return nil
	}

	now := time.Now().Unix()
	models := make([]mongo.WriteModel, 0, len(tds))
	for i := range tds {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": tds[i].ID}).
			SetUpdate(bson.M{"$set": bson.M{"totalQuota": tds[i].TotalQuota.ToData().ToMongo(), "updatedAt": now}}))
	}
	if _, err := c.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameTenantDetail).BulkWrite(ctx, models, nil); err != nil {
		return err
	}

	return nil
}

func InsertTenantQuotaAudit(ctx context.Context, c *mongo.Client, auditEvents []interface{}) error {
	if len(auditEvents) == 0 {
		return nil
	}

	if _, err := c.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColTenantQuotaAudit).InsertMany(ctx, auditEvents); err != nil {
		return err
	}

	return nil
}

func UpdateOrInsertGPUGroup(ctx context.Context, c *mongo.Client, gpuGroups []*quota.GPUGroup) error {
	if len(gpuGroups) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, 2*len(gpuGroups))
	for i := range gpuGroups {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{
				"gpuType": gpuGroups[i].GPUType}).
			SetUpdate(bson.M{
				"$set": bson.M{"gpuTypeLevel": gpuGroups[i].GPUTypeLevel}}).
			SetUpsert(true))
	}
	if _, err := c.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColGPUTypeGroup).BulkWrite(ctx, models, nil); err != nil {
		return err
	}

	return nil
}

type ListGPUGroupParam struct {
	GPUTypeLevel string
}

func ListGPUGroup(ctx *ginlib.GinContext, listParam *ListGPUGroupParam, withoutCount bool) ([]*quota.GPUGroup, int64, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, 0, err
	}

	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColGPUTypeGroup)

	// 同一显卡型号不同驱动类型会同时设置分组，目前默认查看一种驱动类型即可
	filter := bson.M{}
	if listParam.GPUTypeLevel != "" {
		filter["gpuTypeLevel"] = listParam.GPUTypeLevel
	}

	cur, err := col.Find(context.TODO(), filter)
	if err != nil {
		return nil, 0, err
	}
	var res []*quota.GPUGroup
	if err := cur.All(context.TODO(), &res); err != nil {
		return nil, 0, err
	}

	var total int64
	if !withoutCount {
		total, err = col.CountDocuments(context.TODO(), bson.M{}, nil)
		if err != nil {
			return nil, 0, err
		}
	}

	return res, total, nil
}

func DeleteGPUGroupByGPUType(ctx *ginlib.GinContext, gpuTypes []string) error {
	if len(gpuTypes) == 0 {
		return nil
	}

	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}

	_, err = mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColGPUTypeGroup).DeleteMany(
		context.TODO(),
		bson.M{"gpuType": bson.M{"$in": gpuTypes}})
	if err != nil {
		return err
	}

	return nil
}
