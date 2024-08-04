package driver

import "github.com/sarchlab/akita/v4/sim"

// A Middleware is a pluggable element of the driver that can take care of the
// handling of certain types of commands and parts of the driver-GPU
// communication.
type Middleware interface {
	ProcessCommand(
		now sim.VTimeInSec,
		cmd Command,
		queue *CommandQueue,
	) (processed bool)
	Tick(now sim.VTimeInSec) (madeProgress bool)
}
