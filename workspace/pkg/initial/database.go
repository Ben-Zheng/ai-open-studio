package initial

import (
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
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
}

func InitCollection() error {
	gc := ginlib.NewMockGinContext()
	mgoClient, err := consts.MongoClient.GetMongoClient(gc)
	if err != nil {
		return err
	}

	if err := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).CreateCollection(gc, "workspace"); err != nil {
		return err
	}

	if err := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).CreateCollection(gc, "instance"); err != nil {
		return err
	}

	return nil
}
