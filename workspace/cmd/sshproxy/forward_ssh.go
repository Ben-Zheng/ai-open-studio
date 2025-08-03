package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func forwardSSH(ctx context.Context,
	userConn *ssh.ServerConn,
	userChans <-chan ssh.NewChannel,
	userReqs <-chan *ssh.Request,
	user, owner, subject string) {
	// Connect to the real host.
	cconfig := &ssh.ClientConfig{
		User:            owner,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(serverKey)},
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	subjectAddr := fmt.Sprintf("%s:22", subject)
	subjectRawConn, err := net.DialTimeout("tcp", subjectAddr, cconfig.Timeout)
	if err != nil {
		sshErrorf(ctx, userConn, userChans, userReqs, "Dial subject %s: %v", subjectAddr, err)
		return
	}

	// This is the connection facing the subject.
	subjectConn, subjectChans, subjectReqs, err := ssh.NewClientConn(subjectRawConn, subject, cconfig)
	if err != nil {
		sshErrorf(ctx, userConn, userChans, userReqs, "New subject SSH conn %s: %v", subjectAddr, err)
		return
	}
	defer subjectConn.Close()

	go func() {
		if err := forwardConnReqs(userConn, subjectReqs); err != nil {
			log.Debugln("forwardConnReqs", err)
		}
	}()
	go func() {
		if err := forwardConnReqs(subjectConn, userReqs); err != nil {
			log.Debugln("forwardConnReqs", err)
		}
	}()
	go forwardChans(ctx, userConn, subjectChans, user, subject)
	go forwardChans(ctx, subjectConn, userChans, subject, user)

	log.Debugln("Waiting for either connection to close...")
	ctx, cancel := context.WithCancel(ctx)
	go func() { _ = userConn.Wait(); cancel() }()
	go func() { _ = subjectConn.Wait(); cancel() }()
	<-ctx.Done()
	log.Debugln("Connection closed.")
}

func sshErrorf(ctx context.Context, conn *ssh.ServerConn, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request, format string, a ...any) {
	log.Warnf(format, a...)
	if conn == nil {
		log.Warn("conn is nil")
	}

	go ssh.DiscardRequests(reqs)
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case newChan, ok := <-chans:
			if !ok {
				return
			}
			if newChan.ChannelType() != SessionChannelType {
				_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
				continue
			}
			channel, requests, err := newChan.Accept()
			if err != nil {
				return
			}
			go ssh.DiscardRequests(requests)

			bw := bufio.NewWriter(channel.Stderr())
			fmt.Fprintf(bw, "Connection failed: ")
			fmt.Fprintf(bw, format, a...)
			fmt.Fprintf(bw, "\r\n")
			bw.Flush()

			_ = channel.CloseWrite()
			return
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		}
	}
}
