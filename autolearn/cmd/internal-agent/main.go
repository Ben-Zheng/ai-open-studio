package main

import (
	"flag"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/internal-agent/handler"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/internal-agent/thread"
)

func main() {
	flag.Parse()
	internalServer, err := NewServer()
	if err != nil {
		log.WithError(err).Errorf("[agent] new autolearn internal server failed")
		return
	}

	// get autolearn revision from apiserver
	autolearn, err := thread.GetAutolearnRevision()
	if err != nil {
		log.WithError(err).Error("[agent] init interval server err")
		return
	}

	internalServer.RegisterWorker(handler.NewInitWorker(autolearn))
	internalServer.StartUp()
	internalServer.workersWg.Wait()
}
