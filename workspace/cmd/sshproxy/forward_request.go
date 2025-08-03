package main

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func forwardConnReqs(dst ssh.Conn, src <-chan *ssh.Request) error {
	defer log.Debugf("forwardConnReqs done")

	for r := range src {
		ok, data, err := dst.SendRequest(r.Type, r.WantReply, r.Payload)
		log.Debugf("SendConnRequest(%v, %v): %v, %v", r.Type, r.WantReply, ok, err)
		if err != nil {
			return err
		}
		if r.WantReply {
			err := r.Reply(ok, data)
			log.Debugf("ReplyConnRequest(%v): %v, %v", r.Type, ok, err)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func forwardChannelReqs(
	ctx context.Context,
	dst ssh.Channel,
	src <-chan *ssh.Request,
	exitRequestForwarded chan<- struct{}) error {
	defer log.Debugf("forwardChannelReqs done")
	if ctx == nil {
		log.Warn("ctx is nil")
	}

	var receivedExitPrefixedRequest bool
	defer func() {
		if receivedExitPrefixedRequest {
			// According to https://github.com/golang/go/issues/29733
			// Send a signal when exit- prefix request already forwarded.
			// Send signal in non-blocking manner to prevent unexpected blocking.
			select {
			case exitRequestForwarded <- struct{}{}:
			default:
			}
		}
	}()

	for r := range src {
		if strings.HasPrefix(r.Type, "exit-") {
			receivedExitPrefixedRequest = true
		}

		// rsync/scp's payload will passed by "exec" request
		//
		// rsync:
		// if upload, payload will not contain "--sender".
		// if download, payload will contain "--sender".
		// scp:
		// if upload, payload will contain "-t"
		// if download, payload will contain "-f"
		ok, err := dst.SendRequest(r.Type, r.WantReply, r.Payload)
		log.Debugf("SendChannelRequest(%v, %v): %v, %v", r.Type, r.WantReply, ok, err)
		if err != nil {
			return err
		}
		if r.WantReply {
			err := r.Reply(ok, nil)
			log.Debugf("ReplyChannelRequest(%v): %v, %v", r.Type, ok, err)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
