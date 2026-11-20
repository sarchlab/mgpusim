package waferscale

import (
	"github.com/sarchlab/akita/v5/noc/mesh"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// FlitSize is the number of bytes per mesh flit. At 1 GHz, 16-byte flits give
// each link 16 GB/s, Akkalat's default bandwidth.
const FlitSize = 16

// Network places the host and the GPU tiles of a wafer-scale GPU on a 2D
// mesh. Each port joins the endpoint of its device's mesh position.
type Network struct {
	layout    Mesh
	connector *mesh.Connector
}

// NewNetwork builds an empty mesh of the given shape, named "Mesh", with
// 16-byte flits at 1 GHz and the given inter-switch latency in cycles.
func NewNetwork(s modeling.Simulation, m Mesh, switchLatency int) Network {
	return Network{
		layout: m,
		connector: mesh.MakeBuilder().
			WithSimulation(s).
			WithFreq(timing.GHz).
			WithFlitSize(FlitSize).
			WithSwitchLatency(switchLatency).
			Build("Mesh"),
	}
}

// BindHostPort plugs a port of the driver, the IOMMU, or the CPU memory into
// the host position.
func (n Network) BindHostPort(port messaging.Port) {
	n.connector.AddTile(n.layout.HostLocation(), []messaging.Port{port})
}

// BindGPUPort plugs an external port of the GPU with the given 1-based index
// into the position of its tile.
func (n Network) BindGPUPort(index int, port messaging.Port) {
	n.connector.AddTile(n.layout.TileLocation(index), []messaging.Port{port})
}
