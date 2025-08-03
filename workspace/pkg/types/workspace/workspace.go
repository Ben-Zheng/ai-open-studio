package workspace

import (
	"github.com/apex/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

type Workspace struct {
	ID          primitive.ObjectID `bson:"_id" json:"id"`
	UserID      string             `bson:"userID" json:"userID"`
	TenantID    string             `bson:"tenantID" json:"tenantID"`
	ProjectID   string             `bson:"projectID" json:"projectID"`
	Name        string             `bson:"name" json:"name"`
	Runtime     string             `bson:"runtime" json:"runtime"`
	CodeID      string             `bson:"codeID" json:"codeID"` // codebase ID
	Image       string             `bson:"image" json:"image"`   // codebase 对应的 image url
	Spec        *Specification     `bson:"spec" json:"spec"`
	Description string             `bson:"description" json:"description"`
	CreatedBy   string             `bson:"createdBy" json:"createdBy"`
	IsDeleted   bool               `bson:"isDeleted" json:"isDeleted"`
	ExitTimeout int64              `bson:"exitTimeout" json:"exitTimeout"` // 无操作时退出时长，单位：s
	CreatedAt   int64              `bson:"createdAt" json:"createdAt"`
	Level       Level              `bson:"level" json:"level"` // 访问权限

	SSHCMD    string    `bson:"-" json:"sshCMD"`
	Instance  *Instance `bson:"-" json:"instance"`
	ImageName string    `bson:"-" json:"imageName"` // codebase 名
}

type Level string

const (
	ProjectLevel Level = "Project" // 项目共享
	PrivateLevel Level = "Private" // 个人私人
)

type Specification struct {
	CPU      string   `bson:"cpu" json:"cpu"`           // CPU 核数
	MEM      string   `bson:"mem" json:"mem"`           // 内存
	GPU      string   `bson:"gpu" json:"gpu"`           // GPU 卡数
	Storage  string   `bson:"storage" json:"storage"`   // ephemeral-storage 大小
	GPUTypes []string `bson:"gpuTypes" json:"gpuTypes"` // GPU卡类型
}

func GetWorkspaceByID(ctx *ginlib.GinContext, id primitive.ObjectID) (*Workspace, error) {
	var ws Workspace

	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"_id": id, "isDeleted": false}
	if err := col.FindOne(ctx, filter).Decode(&ws); err != nil {
		log.WithError(err).WithField("id", id).Errorf("failed to get ws")
		return nil, err
	}

	return &ws, nil
}

func GetWorkspaceByName(ctx *ginlib.GinContext, userID, projectID, tenantID, name string) (*Workspace, error) {
	var ws Workspace

	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"userID": userID, "projectID": projectID, "tenantID": tenantID, "name": name, "isDeleted": false}
	if err := col.FindOne(ctx, filter).Decode(&ws); err != nil {
		log.WithError(err).WithField("userID", userID).WithField("name", name).Errorf("failed to find ws")
		return nil, err
	}

	return &ws, nil
}

func ListWorkspace(ctx *ginlib.GinContext, isAdmin, onlyMe bool, userID, projectID, tenantID, name string, page, pageSize int, sortBy, order string) (int64, []*Workspace, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"projectID": projectID, "tenantID": tenantID, "isDeleted": false}
	if onlyMe {
		filter["userID"] = userID
	} else if !isAdmin {
		filter["$or"] = []bson.M{{"userID": userID}, {"level": ProjectLevel}}
	}

	if name != "" {
		filter["name"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: name,
				Options: "i",
			},
		}
	}

	sortNum := 1
	if order == consts.SortByDESC {
		sortNum = -1
	}

	findOptions := options.Find()
	findOptions.SetSkip(ctx.GetPagination().Skip)
	findOptions.SetLimit(ctx.GetPagination().Limit)
	findOptions.SetSort(bson.M{sortBy: sortNum})

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	var wss []*Workspace
	if err := cursor.All(ctx, &wss); err != nil {
		return total, nil, err
	}

	return total, wss, nil
}

func ListWorkspaceByUser(ctx *ginlib.GinContext, userID string) ([]*Workspace, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{
		"userID":    userID,
		"tenantID":  ctx.GetAuthTenantID(),
		"isDeleted": false,
	}

	findOptions := options.Find()
	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}

	var wss []*Workspace
	if err := cursor.All(ctx, &wss); err != nil {
		return nil, err
	}
	return wss, nil
}

func CreateWorkspace(ctx *ginlib.GinContext, ws *Workspace) (*Workspace, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	if _, err := col.InsertOne(ctx, ws); err != nil {
		return nil, err
	}

	return ws, nil
}

func DeleteWorkspace(ctx *ginlib.GinContext, id primitive.ObjectID) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"_id": id, "isDeleted": false}
	update := bson.M{
		"$set": bson.M{
			"isDeleted": true,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func UpdateWorkspaceName(ctx *ginlib.GinContext, id primitive.ObjectID, name string) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"_id": id, "isDeleted": false}
	update := bson.M{
		"$set": bson.M{
			"name": name,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func UpdateWorkspaceExitTimeout(ctx *ginlib.GinContext, id primitive.ObjectID, exitTimeout int64) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("workspace")

	filter := bson.M{"_id": id, "isDeleted": false}
	update := bson.M{
		"$set": bson.M{
			"exitTimeout": exitTimeout,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}
