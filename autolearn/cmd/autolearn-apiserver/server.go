package main

import (
	"github.com/gin-contrib/gzip"
	log "github.com/sirupsen/logrus"

	apiV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/hdlr"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginapp"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
)

type AutolearnServer struct {
	*ginapp.App
	mgr *hdlr.Mgr
}

func NewAutoLearnServer(configFile, addr string) *AutolearnServer {
	sentry.InitClient()

	app := ginapp.NewApp(configFile)
	if app == nil {
		log.Error("failed to create gin app")
		return nil
	}

	if app.Conf.ServerAddr == "" {
		log.Warnf("empty server address and default address used: %s", addr)
		app.Conf.ServerAddr = addr
	}
	log.Debugf("listens address is %s", app.Conf.ServerAddr)

	var conf config.Conf
	log.Debugf("start to read config file: %s", configFile)
	if err := utils.LoadFromYAMLFile(configFile, &conf); err != nil {
		log.WithError(err).Error("failed to load server config")
		return nil
	}

	autoLearnMgr, err := hdlr.NewMgr(app, &conf)
	if err != nil {
		log.WithError(err).Error("failed to creat autoLearn manager")
		return nil
	}

	return &AutolearnServer{
		App: app,
		mgr: autoLearnMgr,
	}
}

func (s *AutolearnServer) Run() {
	s.Use(sentry.SetGinCollect(), sentry.SetHubScopeTag(sentry.SeverScopeTag, string(types.AIServiceTypeAutoLearn))).Use(gzip.Gzip(gzip.DefaultCompression))
	s.RunAfterBind(apiV1.Routes(s.mgr))
}
