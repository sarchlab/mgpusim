package driver

import (
	"sync"

	"gitlab.com/akita/mem/vm"
)

// Context is an opaque struct that carries the inforomation used by the driver.
type Context struct {
	pid          vm.PID
	currentGPUID int

	queueMutex sync.Mutex
	queues     []*CommandQueue
}
