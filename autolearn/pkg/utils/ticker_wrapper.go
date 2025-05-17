package utils

import (
	"errors"
	"time"
)

var (
	ErrTickerNotSpecified   = errors.New("ticker callback not specified")
	ErrTickerWrapperStopped = errors.New("ticker wrapper stopped")
)

type TickerWrapper struct {
	ticker *time.Ticker
	stop   chan bool
}

func NewTickerWrapper(duration time.Duration) *TickerWrapper {
	return &TickerWrapper{
		ticker: time.NewTicker(duration),
		stop:   make(chan bool),
	}
}

func (w *TickerWrapper) Range(tickCB func() error) error {
	if tickCB == nil {
		return ErrTickerNotSpecified
	}
	for {
		select {
		case <-w.ticker.C:
			if err := tickCB(); err != nil {
				w.Stop()
				return err
			}
		case <-w.stop:
			return ErrTickerWrapperStopped
		}
	}
}

func (w *TickerWrapper) Stop() {
	if w.IsStop() {
		return
	}

	// stop the runtime timer
	w.ticker.Stop()

	// exit the ticker range loop if have
	SafeCloseBoolChan(w.stop)
}

func (w *TickerWrapper) IsStop() bool {
	return IsClosedBoolChan(w.stop)
}

func IsClosedBoolChan(ch chan bool) bool {
	if ch == nil {
		return true
	}

	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func SafeCloseBoolChan(ch chan bool) {
	if ch == nil {
		return
	}

	select {
	case <-ch:
	default:
		close(ch)
	}
}
