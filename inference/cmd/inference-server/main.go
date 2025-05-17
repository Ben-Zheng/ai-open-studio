package main

import (
	"flag"

	log "github.com/sirupsen/logrus"
)

var (
	flagConf = flag.String("conf", "conf.yaml", "app config file")
)

func main() {
	flag.Parse()

	s := NewInferenceServer(*flagConf)
	if s == nil {
		log.Fatal("new model server failed")
		return
	}
	s.Run()
}
