package arbitration

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
)

// NewXBarArbiter creates a new XBar arbiter.
func NewXBarArbiter() Arbiter {
	return &xbarArbiter{}
}

type xbarArbiter struct {
	buffers    []sim.Buffer
	nextPortID int
}

func (a *xbarArbiter) AddBuffer(buf sim.Buffer) {
	a.buffers = append(a.buffers, buf)
}

func (a *xbarArbiter) Arbitrate(now sim.VTimeInSec) []sim.Buffer {
	startingPortID := a.nextPortID
	selectedPort := make([]sim.Buffer, 0)
	occupiedOutputPort := make(map[sim.Buffer]bool)

	for i := 0; i < len(a.buffers); i++ {
		currPortID := (startingPortID + i) % len(a.buffers)
		buf := a.buffers[currPortID]
		item := buf.Peek()
		if item == nil {
			continue
		}

		flit := item.(*messaging.Flit)
		if _, ok := occupiedOutputPort[flit.OutputBuf]; ok {
			continue
		}

		selectedPort = append(selectedPort, buf)
		occupiedOutputPort[flit.OutputBuf] = true
	}

	a.nextPortID = (a.nextPortID + 1) % len(a.buffers)

	return selectedPort
}
