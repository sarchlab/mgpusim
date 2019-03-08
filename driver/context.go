package driver

import (
	"sync"

	"gitlab.com/akita/mem/vm"
)

type Context struct {
	pid          vm.PID
	currentGPUID int

	queueMutex sync.Mutex
	queues     []*CommandQueue
}
