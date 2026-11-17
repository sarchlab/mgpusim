package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// BuildPageTableMMU builds either role of the timing platform's walker.
// The platform binds the resulting ports to the appropriate memory and peers.
func BuildPageTableMMU(s modeling.Simulation, name string, spec pagetablemmu.Spec) *pagetablemmu.Comp {
	c := modeling.MakeBuilder[*pagetablemmu.Comp](spec).WithSimulation(s).Build(name)
	for _, slot := range []string{"Top", "FaultIn", "PageFault", "Memory", "Control"} {
		c.BindPort(slot, twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).Build())
	}
	return c
}
