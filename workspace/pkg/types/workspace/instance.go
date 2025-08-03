package workspace

import (
	"fmt"
	"time"

	"github.com/go-openapi/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

type Instance struct {
	ID          primitive.ObjectID   `json:"id" bson:"_id"`
	Name        string               `json:"name" bson:"name"`
	Namespace   string               `json:"namespace" bson:"namespace"`
	WorkspaceID primitive.ObjectID   `json:"workspaceID" bson:"workspaceID"`
	State       consts.InstanceState `json:"state" bson:"state"`
	Token       string               `json:"token" bson:"token"`
	LastPingAt  int64                `json:"-" bson:"lastPingAt"` // 上一次查询详情时的时间
	CreatedAt   int64                `json:"createdAt" bson:"createdAt"`
	PodName     string               `json:"podName" bson:"podName"`
	PodIP       string               `json:"podIP" bson:"podIP"`
	NodeName    string               `json:"nodeName" bson:"nodeName"`
	Reason      string               `json:"reason" bson:"reason"`

	UsedDuration int64 `json:"usedDuration" bson:"-"`
}

func GetInstanceName(workspaceID primitive.ObjectID) string {
	return fmt.Sprintf("workspace-%s", workspaceID.Hex())
}

func GetRunningInstanceByWorkspaceIDAndToken(ctx *ginlib.GinContext, workspaceID primitive.ObjectID, token string) (*Instance, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{"workspaceID": workspaceID, "state": consts.InstanceStateRunning, "token": token}
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var inss []Instance
	if err := cursor.All(ctx, &inss); err != nil {
		return nil, err
	}

	if len(inss) == 0 {
		return nil, errors.NotFound("instance")
	}
	return &inss[0], nil
}

func GetInstanceByID(ctx *ginlib.GinContext, id primitive.ObjectID) (*Instance, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{"_id": id}
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var inss []Instance
	if err := cursor.All(ctx, &inss); err != nil {
		return nil, err
	}

	if len(inss) == 0 {
		return nil, errors.NotFound("instance")
	}
	return &inss[0], nil
}

func CreateInstance(ctx *ginlib.GinContext, workspaceID primitive.ObjectID, namespace, token string) (*Instance, error) {
	nIns := Instance{
		ID:          primitive.NewObjectID(),
		WorkspaceID: workspaceID,
		Name:        GetInstanceName(workspaceID),
		Namespace:   namespace,
		State:       consts.InstanceStatePending,
		Token:       token,
		LastPingAt:  time.Now().Unix(),
		CreatedAt:   time.Now().Unix(),
	}

	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	if _, err := col.InsertOne(ctx, nIns); err != nil {
		return nil, err
	}

	return &nIns, nil
}

func UpdateInstance(ctx *ginlib.GinContext, id primitive.ObjectID, ins *Instance) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"podName":  ins.PodName,
			"podIP":    ins.PodIP,
			"nodeName": ins.NodeName,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func SetInstanceLastPingAt(ctx *ginlib.GinContext, id primitive.ObjectID, lastPingAt int64) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"lastPingAt": lastPingAt,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func SetInstanceState(ctx *ginlib.GinContext, id primitive.ObjectID, state consts.InstanceState, reason string) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{"_id": id}

	set := bson.M{"state": state}
	if state == consts.InstanceStateFailed {
		set["reason"] = reason
	}
	update := bson.M{"$set": set}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func ListInstance(ctx *ginlib.GinContext, isStartupOrAll bool, workspaceID primitive.ObjectID, page, pageSize int) (int64, []Instance, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("instance")

	filter := bson.M{}
	if isStartupOrAll {
		filter["state"] = bson.M{"$in": bson.A{consts.InstanceStatePending, consts.InstanceStateRunning}}
	}
	if !workspaceID.IsZero() {
		filter["workspaceID"] = workspaceID
	}

	findOptions := options.Find()
	findOptions.SetSkip(int64((page - 1) * pageSize))
	findOptions.SetLimit(int64(pageSize))
	findOptions.SetSort(bson.M{"createdAt": -1})

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}

	var inss []Instance
	if err := cursor.All(ctx, &inss); err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	return total, inss, nil
}
