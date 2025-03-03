package cu

import (
	"github.com/sarchlab/mgpusim/v4/timing/wavefront"
)

// A SubComponent is an element installed in the compute unit
type SubComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *wavefront.Wavefront)
	Run() bool
	IsIdle() bool
	Flush()
}
