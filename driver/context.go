package driver

import (
	"sync"

	"gitlab.com/akita/util/ca"
)

// Context is an opaque struct that carries the inforomation used by the driver.
type Context struct {
	pid           ca.PID
	currentGPUID  int
	prevPageVAddr uint64

	queueMutex sync.Mutex
	queues     []*CommandQueue
}
