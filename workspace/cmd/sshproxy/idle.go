package main

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

type idleConn struct {
	net.Conn

	ctx            context.Context
	wbytes, rbytes int64
}

// Read implements net.Conn.Read method.
func (c *idleConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	atomic.AddInt64(&c.rbytes, int64(n))
	return
}

// Write implements net.Conn.Write method.
func (c *idleConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	atomic.AddInt64(&c.wbytes, int64(n))
	return
}

// Idle wraps the connection and closes the connection when there is no traffic
// in timeout.
func Idle(ctx context.Context, conn net.Conn, timeout time.Duration) net.Conn {
	c := &idleConn{
		Conn: conn,
		ctx:  ctx,
	}
	go func() {
		t := time.NewTicker(timeout)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				traffic := atomic.SwapInt64(&c.wbytes, 0) + atomic.SwapInt64(&c.rbytes, 0)
				if traffic > 0 {
					continue
				}
				c.Close()
				return
			}
		}
	}()
	return c
}
