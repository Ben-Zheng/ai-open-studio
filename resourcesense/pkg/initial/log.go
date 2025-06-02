package initial

import (
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

func InitLog() {
	if consts.ConfigMap.GinConf.GinMode == "debug" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}
