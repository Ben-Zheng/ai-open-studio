package initial

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo/options"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

func InitDB(conf *consts.Config) {
	var err error

	if consts.MongoClient, err = mgolib.NewMgoClient(conf.MgoConf); err != nil {
		log.WithError(err).Error("new mgo client failed")
		panic(err)
	}

	if err := InitCollection(); err != nil {
		log.WithError(err).Error("failed to InitCollection")
	}
	err = initAuthMongo()
	if err != nil {
		log.WithError(err).Error("failed to Init aisauth  Collection")
		panic(err)
	}
}

func InitCollection() error {
	gc := ginlib.NewMockGinContext()
	mgoClient, err := consts.MongoClient.GetMongoClient(gc)
	if err != nil {
		return err
	}

	database := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName)
	if err := createCollectionIfNotExist(database, []string{"bill", "audit", "node", "resource", "charge", "gpu_type_group", "tenantdetail", "tenant_quota_audit"}); err != nil {
		return err
	}

	return nil
}

func createCollectionIfNotExist(database *mongo.Database, collectionNames []string) error {
	if database == nil {
		return fmt.Errorf("database in nil")
	}

	for i := range collectionNames {
		names, err := database.ListCollectionNames(context.Background(), bson.M{"name": collectionNames[i]})
		if err != nil {
			return err
		}

		if len(names) == 0 {
			log.Debugf("[first time] %s", collectionNames[i])
			if err := database.RunCommand(context.Background(), bson.M{"create": collectionNames[i]}).Err(); err != nil {
				return err
			}
		}
	}

	return nil
}

func initAuthMongo() error {
	connect, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(consts.ConfigMap.AuthConf.MongoURL))
	if err != nil {
		log.WithError(err).Error("CheckAuth failed connect aisauth mongo")
		return err
	}
	consts.AuthMongoClient = connect
	return nil
}
