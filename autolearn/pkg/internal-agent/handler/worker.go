package handler

import (
	"sync"
)

type Worker interface {
	Start(*sync.WaitGroup)
	Stop()
}
