//nolint:govet,errcheck // ssh proxy 不检查
package main

import (
	"context"
	"flag"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	_ "go.megvii-inc.com/brain/brainpp/common/pkg/log"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

var (
	Version = "0.1.0"

	flagBind   = flag.String("bind", ":22", "Bind address")
	flagKey    = flag.String("key", "server.key", "Server SSH key")
	flagServer = flag.String("server", "http://workspace-http.aiservice.svc:8080", "ais server")
	flagNoAuth = flag.Bool("noauth", false, "Allow any user to login for debugging purpose")
	flagDebug  = flag.Bool("debug", false, "Print debug messages")

	serverKey ssh.Signer
)

func main() {
	flag.Parse()

	if *flagDebug {
		log.SetLevel(log.DebugLevel)
	}
	if *flagNoAuth {
		log.Errorln("No auth mode enabled! This flag should not be enabled on production.")
	}
	serverKey = readServerKey()

	var err error

	lis, err := graceListen("tcp", *flagBind)
	if err != nil {
		panic(err)
	}

	log.Infoln("Listening at", *flagBind)
	log.Infof("Graceful restart: kill -SIGUSR2 %d", os.Getpid())
	ctx := context.WithValue(context.Background(), consts.App, "sshproxy")
	var wg sync.WaitGroup
	for {
		tcpConn, err := lis.Accept()
		if err != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			forward(ctx, tcpConn)
		}()
	}
	log.Infof("Waiting for connections to close...")
	go func() {
		time.Sleep(24 * time.Hour)
		log.Infof("24h since restart, terminating now: %d", os.Getpid())
		os.Exit(1)
	}()
	wg.Wait()
	log.Infof("Exited: %d", os.Getpid())
}

func readServerKey() ssh.Signer {
	content, err := os.ReadFile(*flagKey)
	if err != nil {
		panic(err)
	}
	privkey, err := ssh.ParsePrivateKey(content)
	if err != nil {
		panic(err)
	}
	return privkey
}
