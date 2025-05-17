package main

import (
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/recommend-server/api"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/recommend-server/types"
)

var (
	flagConf = flag.String("conf", "config.yaml", "app config file path")
)

func main() {
	flag.Parse()

	cfgf, err := os.ReadFile(*flagConf)
	if err != nil {
		log.WithError(err).Error("read config file failed")
		return
	}
	cfg := types.Config{}
	err = yaml.Unmarshal(cfgf, &cfg)
	if err != nil {
		log.WithError(err).Error("unmarshall config file failed")
		return
	}

	s, err := api.NewRecommendServer()
	if err != nil {
		log.WithError(err).Fatal("new recommend server failed")
		return
	}

	s.Run(cfg.GinConf.ServerAddr)
}
