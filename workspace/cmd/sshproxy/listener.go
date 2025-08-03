package main

import (
	"errors"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultBufSize = 4096
)

type listener struct {
	net.Listener
	sshChans <-chan ssh.NewChannel
}

func newListener(sshChans <-chan ssh.NewChannel) *listener {
	return &listener{
		sshChans: sshChans,
	}
}

func (l *listener) Accept() (net.Conn, error) {
	for newChan := range l.sshChans {
		if newChan.ChannelType() != "direct-tcpip" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type: "+newChan.ChannelType())
			continue
		}
		httpChan, httpReqs, err := newChan.Accept()
		if err != nil {
			return nil, err
		}
		go ssh.DiscardRequests(httpReqs)

		return newConn(httpChan), nil
	}
	return nil, errors.New("listener closed")
}

func (l *listener) Close() error {
	return nil
}

func (l *listener) Addr() net.Addr {
	return &addr{}
}

type conn struct {
	net.Conn
	ch ssh.Channel

	*sync.Cond
	head   *element
	tail   *element
	rtimer *time.Timer // read timer for readDeadline
	rstop  chan int
	rerr   error
}

type element struct {
	buf  []byte
	next *element
}

func newConn(ch ssh.Channel) *conn {
	e := new(element)
	c := &conn{
		ch:     ch,
		Cond:   sync.NewCond(new(sync.Mutex)),
		head:   e,
		tail:   e,
		rtimer: time.NewTimer(time.Hour),
		rstop:  make(chan int),
		rerr:   nil,
	}

	go c.bufferRead()
	go c.readDeadline()
	return c
}

func (c *conn) bufferRead() {
	for {
		buf := make([]byte, defaultBufSize)
		n, err := c.ch.Read(buf)
		if n > 0 {
			c.Cond.L.Lock()
			e := &element{buf: buf[:n]}
			c.tail.next = e
			c.tail = e
			c.Cond.Signal()
			c.Cond.L.Unlock()
		}
		if err != nil {
			c.getReadError(err)
			break
		}
	}
}
func (c *conn) Read(b []byte) (n int, err error) {
	c.Cond.L.Lock()
	defer c.Cond.L.Unlock()

	for len(b) > 0 {
		if len(c.head.buf) > 0 {
			r := copy(b, c.head.buf)
			b, c.head.buf = b[r:], c.head.buf[r:]
			n += r
			continue
		}
		if len(c.head.buf) == 0 && c.head != c.tail {
			c.head = c.head.next
			continue
		}

		if n > 0 {
			break
		}

		c.Cond.Wait()

		if c.rerr != nil {
			err = c.rerr
			c.rerr = nil
			break
		}
	}
	return
}

func (c *conn) Write(b []byte) (n int, err error) {
	return c.ch.Write(b)
}

func (c *conn) Close() error {
	c.stopTimer()
	err := c.ch.Close()
	return err
}

func (c *conn) LocalAddr() net.Addr {
	return &addr{}
}

func (c *conn) RemoteAddr() net.Addr {
	return &addr{}
}

func (c *conn) getReadError(err error) {
	c.Cond.L.Lock()
	c.rerr = err
	c.Cond.Signal()
	c.Cond.L.Unlock()
}

func (c *conn) readDeadline() {
	select {
	case <-c.rtimer.C:
		c.getReadError(&timeoutError{})
	case <-c.rstop:
		return
	}
}

func (c *conn) stopTimer() {
	res := c.rtimer.Stop()
	if res {
		close(c.rstop)
	}
}

func (c *conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		c.stopTimer()
		return nil
	}
	d := time.Until(t)
	c.rtimer.Reset(d)
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return errors.New("not implemented")
}

type addr struct {
}

func (a *addr) Network() string {
	return "ssh-forward"
}

func (a *addr) String() string {
	return "127.0.0.1:8080"
}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
