package arbitration

import (
	"github.com/sarchlab/akita/v3/sim"
)

// Arbiter can determine which buffer can send a message out
type Arbiter interface {
	// Add a buffer for arbitration
	AddBuffer(buf sim.Buffer)

	// Arbitrate returns a set of ports that can send request in the next cycle.
	Arbitrate(now sim.VTimeInSec) []sim.Buffer
}
