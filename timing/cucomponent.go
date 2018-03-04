package timing

import "gitlab.com/yaotsu/core"

// A CUComponent is an element installed in the compute unit
type CUComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *Wavefront, now core.VTimeInSec)
	Run(now core.VTimeInSec)
}
