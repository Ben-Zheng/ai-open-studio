package resource

import (
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

const (
	MaxRunTime     int64 = 24 * 60 * 60
	RecentInterval int64 = 7 * 24 * 60 * 60
)

func BillSync() {
	correctBill()
	var interval int64 = 30
	t := time.NewTicker(time.Second * time.Duration(interval))
	for {
		<-t.C
		log.Info("start BillSync")
		if err := syncBill(interval); err != nil {
			log.WithError(err).Error("failed to syncBill")
		}
	}
}

func correctBill() {
	gc := ginlib.NewMockGinContext()
	if err := CorrectBill(gc); err != nil {
		log.WithError(err).Error("failed to CorrectBill")
	}
}

func syncBill(interval int64) error {
	gc := ginlib.NewMockGinContext()
	page := 1
	pageSize := 100

	for {
		_, res, err := ListAllResource(gc, page, pageSize, true)
		if err != nil {
			log.WithError(err).Error("failed to getUserList")
			return err
		}

		for i := range res {
			if res[i].UsedQuota.CPU == 0 && res[i].UsedQuota.GPU == 0 && res[i].UsedQuota.VirtGPU == 0 && res[i].UsedQuota.HDD == 0 && res[i].UsedQuota.Memory == 0 && res[i].UsedQuota.SSD == 0 {
				log.Debug("this resource not be used")
				continue
			}

			bi, err := GetBillByUserIDAndTenantID(gc, res[i].UserID, res[i].TenantID)
			if err != nil {
				log.WithError(err).Error("failed to GetBillByUserIDAndTenantID")
				bi = &resource.Bill{
					UserID:      res[i].UserID,
					TenantID:    res[i].TenantID,
					CreatedAt:   time.Now().Unix(),
					UpdateAt:    time.Now().Unix(),
					IsCorrected: true,
				}
				if err := CreateBill(gc, bi); err != nil {
					log.WithError(err).Error("failed to CreateBill")
					return err
				}
			}

			now := time.Now().Unix()
			_, nchs, err := ListCharge(gc, 1, 1, 0, now, res[i].UserID, res[i].TenantID)
			if err != nil {
				log.WithError(err).Error("failed to ListCharge")
			}

			runTime := MaxRunTime
			if len(nchs) == 0 {
				runTime = interval
			} else if now-nchs[0].Timestamp < runTime {
				runTime = now - nchs[0].Timestamp
			}

			if err := CreateCharge(gc, &resource.Charge{
				UserID:    res[i].UserID,
				TenantID:  res[i].TenantID,
				UsedQuota: res[i].UsedQuota,
				RunTime:   runTime,
				Timestamp: now,
			}); err != nil {
				log.WithError(err).Error("failed to CreateCharge")
				continue
			}

			recent := now - RecentInterval
			_, dch, err := ListCharge(gc, 0, 0, 0, recent, "", "")
			if err != nil {
				log.WithError(err).Error("failed to ListCharge")
				continue
			}

			for index := range dch {
				if err := DeleteCharge(gc, &dch[index]); err != nil {
					log.WithError(err).Error("failed to DeleteCharge")
					continue
				}
			}

			cpu := bi.Accumulate.CPU + res[i].UsedQuota.CPU*float64(runTime)
			gpu := bi.Accumulate.GPU + res[i].UsedQuota.GPU*float64(runTime)
			virtgpu := bi.Accumulate.VirtGPU + res[i].UsedQuota.VirtGPU*float64(runTime)
			mem := bi.Accumulate.Memory + res[i].UsedQuota.Memory*float64(runTime)
			hdd := bi.Accumulate.HDD + res[i].UsedQuota.HDD*float64(runTime)
			ssd := bi.Accumulate.SSD + res[i].UsedQuota.SSD*float64(runTime)
			acpu := int64(cpu / consts.AccumulateAdvanceCPU)
			agpu := int64(gpu / consts.AccumulateAdvanceGPU)
			vagpu := int64(virtgpu / consts.AccumulateAdvanceGPU)
			amem := int64(mem / consts.AccumulateAdvanceMemory)
			ahdd := int64(hdd / consts.AccumulateAdvanceHDD)
			assd := int64(ssd / consts.AccumulateAdvanceSSD)

			bi.Accumulate.CPU = cpu - float64(acpu*consts.AccumulateAdvanceCPU)
			bi.Accumulate.GPU = gpu - float64(agpu*consts.AccumulateAdvanceGPU)
			bi.Accumulate.VirtGPU = virtgpu - float64(vagpu*consts.AccumulateAdvanceGPU)
			bi.Accumulate.Memory = mem - float64(amem*consts.AccumulateAdvanceMemory)
			bi.Accumulate.HDD = hdd - float64(ahdd*consts.AccumulateAdvanceHDD)
			bi.Accumulate.SSD = ssd - float64(assd*consts.AccumulateAdvanceSSD)
			bi.AccumulateAdvance.CPU += float64(acpu)
			bi.AccumulateAdvance.GPU += float64(agpu)
			bi.AccumulateAdvance.VirtGPU += float64(vagpu)
			bi.AccumulateAdvance.Memory += float64(amem)
			bi.AccumulateAdvance.HDD += float64(ahdd)
			bi.AccumulateAdvance.SSD += float64(assd)

			if err := UpdateBill(gc, bi); err != nil {
				log.WithError(err).Error("failed to UpdateBill")
				continue
			}
		}

		if len(res) < pageSize {
			break
		}
		page++
	}

	return nil
}

func GetBillByGroup(ctx *ginlib.GinContext, groupBy, userID, tenantID string) (*resource.Bill, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("bill")

	filter := bson.M{}
	if groupBy == consts.CategoryUser {
		filter["userID"] = userID
	} else if groupBy == consts.CategoryTenant {
		filter["tenantID"] = tenantID
	}

	group := primitive.M{
		"_id":                       nil,
		"accumulate_cpu":            primitive.M{"$sum": "$accumulate.cpu"},
		"accumulate_memory":         primitive.M{"$sum": "$accumulate.memory"},
		"accumulate_gpu":            primitive.M{"$sum": "$accumulate.gpu"},
		"accumulate_virtgpu":        primitive.M{"$sum": "$accumulate.virtgpu"},
		"accumulate_ssd":            primitive.M{"$sum": "$accumulate.ssd"},
		"accumulate_hdd":            primitive.M{"$sum": "$accumulate.hdd"},
		"accumulate_advance_cpu":    primitive.M{"$sum": "$accumulateAdvance.cpu"},
		"accumulate_advance_memory": primitive.M{"$sum": "$accumulateAdvance.memory"},
		"accumulate_advance_gpu":    primitive.M{"$sum": "$accumulateAdvance.gpu"},
		"accumulate_advance_ssd":    primitive.M{"$sum": "$accumulateAdvance.ssd"},
		"accumulate_advance_hdd":    primitive.M{"$sum": "$accumulateAdvance.hdd"},
	}

	project := primitive.M{
		"accumulate": primitive.M{
			"cpu":     "$accumulate_cpu",
			"memory":  "$accumulate_memory",
			"gpu":     "$accumulate_gpu",
			"virtgpu": "$accumulate_virtgpu",
			"ssd":     "$accumulate_ssd",
			"hdd":     "$accumulate_hdd",
		},
		"accumulateAdvance": primitive.M{
			"cpu":     "$accumulate_advance_cpu",
			"memory":  "$accumulate_advance_memory",
			"gpu":     "$accumulate_advance_gpu",
			"virtgpu": "$accumulate_advance_virtgpu",
			"ssd":     "$accumulate_advance_ssd",
			"hdd":     "$accumulate_advance_hdd",
		},
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{"$group": group},
		{"$project": project},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var bs []resource.Bill
	if err := cursor.All(ctx, &bs); err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return &resource.Bill{}, nil
	}

	return &bs[0], nil
}

func GetBillByUserIDAndTenantID(ctx *ginlib.GinContext, userID, tenantID string) (*resource.Bill, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("bill")

	filter := bson.M{
		"userID":   userID,
		"tenantID": tenantID,
	}

	var bi resource.Bill
	if err := col.FindOne(ctx, filter).Decode(&bi); err != nil {
		return nil, err
	}

	return &bi, nil
}

func UpdateBill(ctx *ginlib.GinContext, bi *resource.Bill) error {
	updateAt := time.Now().Unix()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("bill")

	filter := bson.M{"userID": bi.UserID, "tenantID": bi.TenantID}
	update := bson.M{
		"$set": bson.M{
			"accumulate":        bi.Accumulate,
			"accumulateAdvance": bi.AccumulateAdvance,
			"updateAt":          updateAt,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func CorrectBill(ctx *ginlib.GinContext) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("bill")

	filter := bson.M{"isCorrected": bson.M{"$exists": false}}
	update := bson.M{
		"$set": bson.M{
			"isCorrected": true,
		},
		"$mul": bson.M{
			"accumulate.cpu":            30,
			"accumulate.memory":         30,
			"accumulate.gpu":            30,
			"accumulate.virtgpu":        30,
			"accumulate.ssd":            30,
			"accumulate.hdd":            30,
			"accumulateAdvance.cpu":     30,
			"accumulateAdvance.memory":  30,
			"accumulateAdvance.gpu":     30,
			"accumulateAdvance.virtgpu": 30,
			"accumulateAdvance.ssd":     30,
			"accumulateAdvance.hdd":     30,
		},
	}

	if _, err := col.UpdateMany(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func CreateBill(ctx *ginlib.GinContext, bi *resource.Bill) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("bill")

	if _, err := col.InsertOne(ctx, bi); err != nil {
		return err
	}

	return nil
}

func DeleteCharge(ctx *ginlib.GinContext, ch *resource.Charge) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("charge")
	filter := bson.M{
		"userID":    ch.UserID,
		"tenantID":  ch.TenantID,
		"timestamp": ch.Timestamp,
	}
	if _, err := col.DeleteOne(ctx, filter); err != nil {
		return err
	}

	return nil
}

func CreateCharge(ctx *ginlib.GinContext, ch *resource.Charge) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("charge")

	if _, err := col.InsertOne(ctx, ch); err != nil {
		return err
	}

	return nil
}

func ListCharge(ctx *ginlib.GinContext, page, pageSize int, startAt, endAt int64, userID, tenantID string) (int64, []resource.Charge, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("charge")

	filter := bson.M{}
	timestamp := bson.M{}
	if startAt > 0 {
		timestamp["$gt"] = startAt
	}
	if endAt > 0 {
		timestamp["$lt"] = endAt
	}
	if len(timestamp) > 0 {
		filter["timestamp"] = timestamp
	}
	if userID != "" {
		filter["userID"] = userID
	}
	if tenantID != "" {
		filter["tenantID"] = tenantID
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.M{"timestamp": -1})
	if pageSize > 0 {
		findOptions.SetSkip(int64((page - 1) * pageSize))
		findOptions.SetLimit(int64(pageSize))
	}

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var cs []resource.Charge
	if err := cursor.All(ctx, &cs); err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	return total, cs, nil
}

func GetRecentAccumulate(ctx *ginlib.GinContext, groupBy, userID, tenantID string) (*resource.Charge, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("charge")

	filter := bson.M{}
	if groupBy == consts.CategoryUser {
		filter["userID"] = userID
	} else if groupBy == consts.CategoryTenant {
		filter["tenantID"] = tenantID
	}

	group := primitive.M{
		"_id":     nil,
		"cpu":     primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.cpu", "$runTime"}}},
		"memory":  primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.memory", "$runTime"}}},
		"gpu":     primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.gpu", "$runTime"}}},
		"virtgpu": primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.virtgpu", "$runTime"}}},
		"ssd":     primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.ssd", "$runTime"}}},
		"hdd":     primitive.M{"$sum": primitive.M{"$multiply": primitive.A{"$usedQuota.hdd", "$runTime"}}},
	}

	project := primitive.M{
		"usedQuota": primitive.M{
			"cpu":     "$cpu",
			"memory":  "$memory",
			"gpu":     "$gpu",
			"virtgpu": "$virtgpu",
			"ssd":     "$ssd",
			"hdd":     "$hdd",
		},
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{"$group": group},
		{"$project": project},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var cs []resource.Charge
	if err := cursor.All(ctx, &cs); err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return &resource.Charge{}, nil
	}

	return &cs[0], nil
}
