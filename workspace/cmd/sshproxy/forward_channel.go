package main

import (
	"context"
	"io"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

const SessionChannelType = "session"

type ChannelOpener interface {
	OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error)
}

func forwardChans(ctx context.Context, dst ChannelOpener, chans <-chan ssh.NewChannel, dstSubject, srcSubject string) {
	for newChan := range chans {
		go forwardChan(ctx, dst, newChan, dstSubject, srcSubject)
	}
}

func forwardChan(ctx context.Context, dst ChannelOpener, newChan ssh.NewChannel, dstSubject, srcSubject string) {
	log.Debugln("forwardChan", newChan.ChannelType())
	defer log.Debugln("forwarcChan done", newChan.ChannelType())

	dstChan, dstReqs, err := dst.OpenChannel(newChan.ChannelType(), newChan.ExtraData())
	if err != nil {
		log.Debugln("OpenChannel:", err)
		if err := newChan.Reject(ssh.Prohibited, err.Error()); err != nil {
			log.Debugln("Reject", err)
		}
		return
	}
	defer dstChan.Close()

	srcChan, srcReqs, err := newChan.Accept()
	if err != nil {
		log.Debugln("Accept:", err)
		return
	}
	defer srcChan.Close()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return copyWithReqs(ctx, srcChan, dstChan, dstReqs, srcSubject, newChan.ChannelType() == SessionChannelType)
	})
	g.Go(func() error {
		return copyWithReqs(ctx, dstChan, srcChan, srcReqs, dstSubject, newChan.ChannelType() == SessionChannelType)
	})

	if err := g.Wait(); err != nil {
		log.Debugln("Wait", err)
	}
}

func copyWithReqs(ctx context.Context, dst, src ssh.Channel, srcReqs <-chan *ssh.Request, subject string, isSession bool) error {
	// According to https://github.com/golang/go/issues/29733
	// Before we close the channel, We have to wait until exit- prefixed request forwarded.
	// forwardChannelReqs should notify when it after forward exit- prefixed request.
	// io.Copy may encounter error and exit early (do not consume the channel), so we have to leave a slot in it.
	exitRequestForwarded := make(chan struct{}, 1)

	g, ctx := errgroup.WithContext(ctx)
	go func() { <-ctx.Done(); dst.Close() }()
	g.Go(func() error { return forwardChannelReqs(ctx, dst, srcReqs, exitRequestForwarded) })
	g.Go(func() error {
		// only log channels with type `session`
		_, err := io.Copy(dst.Stderr(), src.Stderr())
		return err
	})
	g.Go(func() error {
		_, err := io.Copy(dst, src)
		switch err {
		case nil:
			// When receiving EOF (which means io.Copy returns nil), wait exit- prefixed request forwarded before we close channel.
			// For more detail, see https://github.com/golang/go/issues/29733
			t := time.NewTimer(time.Second)
			defer t.Stop()
			select {
			case <-t.C:
				// We can't wait forever, exit anyway.
			case <-exitRequestForwarded:
				// Already forwarded
			}
		default:
			// Encounter error, Don't need to wait anything, Close immediately.
		}

		if err := dst.CloseWrite(); err != nil {
			log.WithField("subject", subject).Debugln("CloseWrite", err)
		}
		return err
	})
	return g.Wait()
}
