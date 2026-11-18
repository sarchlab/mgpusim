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

// BuildLocalPageTableMMU connects the walker's write geometry to the GPU L2.
// A smaller explicit transaction limit still applies. Invalid limits are
// passed to the MMU's ordinary configuration validation.
func BuildLocalPageTableMMU(s modeling.Simulation, name string, spec pagetablemmu.Spec,
	log2CacheLineSize uint64,
) *pagetablemmu.Comp {
	if log2CacheLineSize < 6 {
		lineBytes := uint64(1) << log2CacheLineSize
		switch spec.MaxZeroWriteSize {
		case 0:
			spec.MaxZeroWriteSize = lineBytes
		case 8, 16, 32, 64:
			spec.MaxZeroWriteSize = min(spec.MaxZeroWriteSize, lineBytes)
		}
	}
	return BuildPageTableMMU(s, name, spec)
}
