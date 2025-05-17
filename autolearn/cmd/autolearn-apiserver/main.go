package main

import (
	"flag"

	log "github.com/sirupsen/logrus"

	autoLearnMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
)

var (
	flagConf = flag.String("conf", "./projects/aiservice/autolearn/cmd/autolearn-apiserver/conf.yaml", "app config file")
	flagAddr = flag.String("addr", "0.0.0.0:8080", "server addr")
)

// @title Swagger AutoLearn API
// @version 0.0.1
// @description autoLearn api server
// @termsOfService https://discourse.brainpp.cn/c/support

// @contact.name Brainpp Support
// @contact.url https://discourse.brainpp.cn/c/support

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host autolearn.brainpp.cn
// @BasePath /v1/
// @query.collection.format multi

func main() {
	flag.Parse()

	s := NewAutoLearnServer(*flagConf, *flagAddr)
	if s == nil {
		log.Fatal("new autoLearn server failed")
		return
	}

	if features.IsKubebrainRuntime() {
		go autoLearnMgr.SyncToES(s.App.MgoClient)
		go autoLearnMgr.SyncTransformerToES(s.App.MgoClient)
	}

	s.Run()
}
