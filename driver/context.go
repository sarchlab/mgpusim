package driver

import (
	"sync"

	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

type buffer struct {
	vAddr Ptr
	size  uint64
	freed bool

	// After a kernel is launched, the l2 cache contain dirty data that belongs
	// to this buffer. Therefore, copying from or to this buffer triggers L2
	// flushing.
	l2Dirty bool
}

// Context is an opaque struct that carries the information used by the driver.
type Context struct {
	pid           vm.PID
	currentGPUID  int
	prevPageVAddr uint64
	l2Dirty       bool

	queueMutex sync.Mutex
	queues     []*CommandQueue

	buffers []*buffer
}

func (c *Context) markAllBuffersDirty() {
	for _, b := range c.buffers {
		b.l2Dirty = true
	}
}

func (c *Context) markAllBuffersClean() {
	for _, b := range c.buffers {
		b.l2Dirty = false
	}
}

func (c *Context) removeFreedBuffers() {
	for i, b := range c.buffers {
		if b.freed {
			c.buffers = append(c.buffers[:i], c.buffers[i+1:]...)
		}
	}
}
