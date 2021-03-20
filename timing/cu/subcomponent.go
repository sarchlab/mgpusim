package cu

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/timing/wavefront"
)

// A SubComponent is an element installed in the compute unit
type SubComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *wavefront.Wavefront, now sim.VTimeInSec)
	Run(now sim.VTimeInSec) bool
	IsIdle() bool
	Flush()
}
