package main

import (
	"flag"

	_ "go.megvii-inc.com/brain/brainpp/common/pkg/log"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/controller/heartbeat"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/initial"
)

// configPath 配置文件路径
var configPath string

// etcdCertPath etct证书路径
var etcdCertPath string

func init() {
	flag.StringVar(&configPath, "config", "./projects/aiservice/workspace/hack/workspace.yaml", "Config file for public service")
}

func main() {
	flag.Parse()

	sentry.InitClient()
	initial.InitConfig(configPath)
	initial.InitDB(consts.ConfigMap)
	initial.InitClient()
	initial.InitRouter(consts.ConfigMap.GinConf)

	go heartbeat.Detect()

	consts.GinEngine.Run()
}
