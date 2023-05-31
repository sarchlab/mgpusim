package standalone

import (
	"math/rand"

	"github.com/sarchlab/akita/v3/sim"
)

// A TrafficInjector can inject traffic to a network.
type TrafficInjector interface {
	RegisterAgent(a *Agent)
	InjectTraffic()
}

// GreedyTrafficInjector generate a large number of traffic at the beginning of
// the simulation.
type GreedyTrafficInjector struct {
	agents []*Agent

	engine sim.Engine

	PacketSize int
	NumPackets int
}

// NewGreedyTrafficInjector creates a new GreedyTrafficInjector.
func NewGreedyTrafficInjector(engine sim.Engine) *GreedyTrafficInjector {
	ti := new(GreedyTrafficInjector)
	ti.PacketSize = 1024
	ti.NumPackets = 1024
	ti.engine = engine
	return ti
}

// RegisterAgent allows the GreedyTraffiCInjector to inject traffic from the
// agent.
func (ti *GreedyTrafficInjector) RegisterAgent(a *Agent) {
	ti.agents = append(ti.agents, a)
}

// InjectTraffic injects traffic.
func (ti *GreedyTrafficInjector) InjectTraffic() {
	for i, a := range ti.agents {
		for j := 0; j < ti.NumPackets; j++ {
			dstID := rand.Int() % (len(ti.agents) - 1)
			if dstID >= i {
				dstID++
			}
			dst := ti.agents[dstID]

			pkt := NewStartSendEvent(0, a, dst, ti.PacketSize, j)
			ti.engine.Schedule(pkt)
		}
	}
}
