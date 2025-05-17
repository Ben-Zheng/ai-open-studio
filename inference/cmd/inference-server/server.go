package main

import (
	log "github.com/sirupsen/logrus"

	v1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/mgr"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginapp"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
)

type InferenceServer struct {
	*ginapp.App
	mgr *mgr.Mgr
}

func NewInferenceServer(configFile string) *InferenceServer {
	sentry.InitClient()

	app := ginapp.NewApp(configFile)
	if app == nil {
		log.Error("new app failed")
		return nil
	}

	var config mgr.Config
	log.Debugf("start to read config file: %s", configFile)
	if err := utils.LoadFromYAMLFile(configFile, &config); err != nil {
		log.WithError(err).Error("failed to load server config")
		return nil
	}

	if lv, err := log.ParseLevel(config.LogLevel); err == nil {
		log.SetLevel(lv)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	modelMgr, err := mgr.NewMgr(app, &config)
	if err != nil {
		log.WithError(err).Error("failed to create mgr")
		return nil
	}
	return &InferenceServer{
		App: app,
		mgr: modelMgr,
	}
}

func (s *InferenceServer) Run() {
	s.Use(sentry.SetGinCollect(), sentry.SetHubScopeTag(sentry.SeverScopeTag, string(types.AIServiceTypeInference)))
	go s.mgr.Run()
	s.RunAfterBind(v1.Routes(s.mgr))
}
