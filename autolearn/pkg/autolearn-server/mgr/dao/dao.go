package dao

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
)

const (
	COLNameAutoLearn          string = "autolearn"
	colAutoLearnAlgorithm     string = "algorithm"
	colAutoLearnEvaluation    string = "evaluation"
	colEvalComparison         string = "eval_comparison"
	colEvalComparisonSnapshot string = "eval_comparison_snapshot"
	colAutoLearnSuffix        string = "-autolearn"

	// TODO 待格式统一
	filedID                            string = "_id"
	filedTenantID                      string = "tenantID"
	filedProjectID                     string = "projectID"
	filedProjectName                   string = "projectName"
	filedAutoLearnID                   string = "autoLearnID"
	filedAutoLearnName                 string = "autoLearnName"
	filedCreatedByName                 string = "createdBy.userName"
	filedCreatedByID                   string = "createdBy.userID"
	filedTags                          string = "tags"
	filedUpdatedAt                     string = "updatedAt"
	filedCreatedAt                     string = "createdAt"
	filedIsDeleted                     string = "isDeleted"
	filedAutoLearnRevisions            string = "autoLearnRevisions"
	filedAutoLearnRevisionsID          string = "autoLearnRevisions.revisionID"
	filedAutoLearnRevisionsState       string = "autoLearnRevisions.autoLearnState"
	filedAutoLearnRevisionsCreatedByID string = "autoLearnRevisions.createdBy.userID"
	filedRevisionID                    string = "revisionID"
	filedRevisionName                  string = "revisionName"
	filedAutoLearnState                string = "autoLearnState"
	filedAutoLearnRvReason             string = "reason"
	filedEvalDetSdsFilePath            string = "sdsFilePath"
	filedEvaluationID                  string = "evaluationID"
	filedEvaluationName                string = "evaluationName"
	filedEvaluationJobs                string = "evaluationJobs"
	filedEvaluationJobID               string = "evaluationJobID"
	filedEvaluationJobName             string = "evaluationRJobName"
	filedEvaluationJobState            string = "evaluationJobState"
	filedEvalDetModelPath              string = "modelFilePath"
	filedRevisionEarlyStop             string = "earlyStop"
	filedRvReLabelTaskID               string = "reLabelTaskID"
	filedRvIsHidden                    string = "isHidden"
)

type BasicInfo struct {
	Project     string
	AutoLearnID string
	RevisionID  string
}

type DAO struct {
	Collections *Collections
	TenantID    string
	Context     context.Context
	Cancel      context.CancelFunc
}

type Collections struct {
	AutoLearn, AutoLearnAlgorithm, AutoLearnEvaluation, EvalComparison, ComparisonSnapshot ParserServiceCollectionGetter
}

type ParserServiceCollectionGetter func() *mongo.Collection

func NewDAO(client *mgolib.MgoClient, tenant string) *DAO {
	initialAllCollections(client, tenant)
	return &DAO{
		Collections: &Collections{
			AutoLearn:           func() *mongo.Collection { return GetAutoLearnCol(client, tenant) },
			AutoLearnAlgorithm:  func() *mongo.Collection { return GetAlgorithmCol(client, tenant) },
			AutoLearnEvaluation: func() *mongo.Collection { return GetEvaluationCol(client, tenant) },
			EvalComparison:      func() *mongo.Collection { return GetEvalComparisonCol(client, tenant) },
			ComparisonSnapshot:  func() *mongo.Collection { return GetComparisonSnapshotCol(client, tenant) },
		},
		Context:  context.TODO(),
		TenantID: tenant,
	}
}

func GetAutoLearnCol(client *mgolib.MgoClient, tenant string) *mongo.Collection {
	return client.GetCollection(getMockGinContext(tenant), COLNameAutoLearn, nil)
}

func GetAlgorithmCol(client *mgolib.MgoClient, tenant string) *mongo.Collection {
	return client.GetCollection(getMockGinContext(tenant), colAutoLearnAlgorithm, nil)
}

func GetEvaluationCol(client *mgolib.MgoClient, tenant string) *mongo.Collection {
	return client.GetCollection(getMockGinContext(tenant), colAutoLearnEvaluation, nil)
}

func GetEvalComparisonCol(client *mgolib.MgoClient, tenant string) *mongo.Collection {
	return client.GetCollection(getMockGinContext(tenant), colEvalComparison, nil)
}

func GetComparisonSnapshotCol(client *mgolib.MgoClient, tenant string) *mongo.Collection {
	return client.GetCollection(getMockGinContext(tenant), colEvalComparisonSnapshot, nil)
}

func initialAllCollections(client *mgolib.MgoClient, tenant string) {
	ctx := getMockGinContext(tenant)
	c, err := client.GetMongoClient(ctx)
	if err != nil {
		return
	}

	database := c.Database(client.Conf.MongoDBName)

	initCol(database, client.GetCollectionName(ctx, COLNameAutoLearn),
		[]mongo.IndexModel{
			{
				Keys: bson.D{
					{Key: filedProjectID, Value: -1},
					{Key: filedTenantID, Value: -1},
					{Key: filedAutoLearnName, Value: -1}},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(false),
			},
			{
				Keys: bson.M{filedAutoLearnID: -1},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(true),
			},
			{
				Keys: bson.M{filedUpdatedAt: -1},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(false),
			},
		},
	)

	initCol(database, client.GetCollectionName(ctx, colAutoLearnAlgorithm),
		[]mongo.IndexModel{
			{
				Keys: bson.D{
					{Key: filedRevisionID, Value: -1},
					{Key: filedAutoLearnID, Value: -1},
				},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(true),
			},
		},
	)

	initCol(database, client.GetCollectionName(ctx, colAutoLearnEvaluation),
		[]mongo.IndexModel{
			{
				Keys: bson.D{
					{Key: filedRevisionID, Value: -1},
					{Key: filedAutoLearnID, Value: -1},
				},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(true),
			},
		},
	)

	initCol(database, client.GetCollectionName(ctx, colEvalComparisonSnapshot),
		[]mongo.IndexModel{
			{
				Keys: bson.M{filedUpdatedAt: -1},
				// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
				Options: options.Index().SetBackground(true).SetUnique(false),
			},
		})
}

func initCol(database *mongo.Database, colName string, indexes []mongo.IndexModel) {
	if _, loaded := config.ColInitMarkers.LoadOrStore(colName, true); !loaded {
		log.Debugf("just only once: %s", colName)
		if err := createCollectionIfNotExist(database, colName); err != nil {
			log.WithError(err).Errorf("failed to create collection: %s", colName)
			return
		}

		if err := createIndexes(database.Collection(colName), indexes); err != nil {
			log.WithError(err).Error("failed to create index")
			return
		}
	}
}

func createCollectionIfNotExist(database *mongo.Database, collectionName string) error {
	if database == nil {
		return fmt.Errorf("database in nil")
	}

	names, err := database.ListCollectionNames(context.Background(), bson.M{"name": collectionName})
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Debugf("[first time] %s", collectionName)
		if err := database.RunCommand(context.Background(), bson.M{"create": collectionName}).Err(); err != nil {
			return err
		}
	}

	return nil
}

// createIndexes 创建索引
func createIndexes(col *mongo.Collection, indexes []mongo.IndexModel) error {
	if len(indexes) == 0 {
		return nil
	}

	if _, err := col.Indexes().CreateMany(context.Background(), indexes); err != nil {
		log.WithError(err).Errorf("failed to create index: %+v", indexes)
		return err
	}

	return nil
}

func getMockGinContext(tenant string) *ginlib.GinContext {
	gc := ginlib.NewMockGinContext()
	gc.SetTenantName(tenant)
	return gc
}

func GetAllTenantName(client *mgolib.MgoClient) ([]string, error) {
	colNames, err := client.GetCollectionNames()
	if err != nil {
		return nil, err
	}

	tenantNames := make([]string, 0)
	for i := range colNames {
		if strings.HasSuffix(colNames[i], colAutoLearnSuffix) {
			tenantName := strings.TrimSuffix(colNames[i], colAutoLearnSuffix)
			tenantNames = append(tenantNames, tenantName)
		}
	}

	return tenantNames, err
}
