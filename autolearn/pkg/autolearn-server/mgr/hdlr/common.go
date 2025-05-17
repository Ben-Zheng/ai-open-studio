package hdlr

import (
	"fmt"
	// log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

func getAutoLearnAndRevisionID(gc *ginlib.GinContext) (string, string, error) {
	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		return "", "", fmt.Errorf("invalid param autoLearnID")
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		return "", "", fmt.Errorf("invalid param revisionID")
	}

	return autoLearnID, revisionID, nil
}
