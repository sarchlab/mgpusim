package cu

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/timing/wavefront"
)

// A SubComponent is an element installed in the compute unit
type SubComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *wavefront.Wavefront, now sim.VTimeInSec)
	Run(now sim.VTimeInSec) bool
	IsIdle() bool
	Flush()
}
