package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/facebookgo/grace/gracenet"
	proxyproto "github.com/pires/go-proxyproto"
	log "github.com/sirupsen/logrus"
)

var (
	gnet       = &gracenet.Net{}
	didInherit = os.Getenv("LISTEN_FDS") != ""
	ppid       = os.Getppid()
)

func graceListen(nett string, bind string) (net.Listener, error) {
	lis, err := gnet.Listen(nett, bind)
	if err != nil {
		return nil, err
	}
	lis = &proxyproto.Listener{Listener: lis}

	ch := make(chan os.Signal, 10)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR2)
	go func() {
		for {
			sig := <-ch
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				// This ensures a subsequent INT/TERM will trigger standard go behavior of
				// terminating.
				lis.Close()
				return
			case syscall.SIGUSR2:
				// We only return here if there's an error, otherwise the new process
				// will send us a TERM when it's ready to trigger the actual shutdown.
				if _, err := gnet.StartProcess(); err != nil {
					log.WithError(err).Warn("StartProcess")
				}
			}
		}
	}()

	// Close the parent if we inherited and it wasn't init that started us.
	if didInherit && ppid != 1 {
		if err := syscall.Kill(ppid, syscall.SIGTERM); err != nil {
			return nil, fmt.Errorf("failed to close parent: %s", err)
		}
	}

	return lis, err
}
