package timing

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/wavefront"
)

//go:generate mockgen -destination mock_timing/cucomponent.go gitlab.com/akita/gcn3/timing CUComponent

// A CUComponent is an element installed in the compute unit
type CUComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *wavefront.Wavefront, now akita.VTimeInSec)
	Run(now akita.VTimeInSec) bool
	IsIdle() bool
	Flush()
}
