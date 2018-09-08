package timing

import "gitlab.com/akita/akita"

// A CUComponent is an element installed in the compute unit
type CUComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *Wavefront, now akita.VTimeInSec)
	Run(now akita.VTimeInSec)
}
