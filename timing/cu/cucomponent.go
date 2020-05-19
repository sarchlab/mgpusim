package cu

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/timing/wavefront"
)

// A CUComponent is an element installed in the compute unit
type CUComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *wavefront.Wavefront, now akita.VTimeInSec)
	Run(now akita.VTimeInSec) bool
	IsIdle() bool
	Flush()
}
