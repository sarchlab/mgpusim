package timingconfig

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

// interDeviceNetwork connects the host to the GPUs and the GPUs to each
// other. It tells host ports from GPU ports so that a topology can place each
// device.
type interDeviceNetwork interface {
	// BindHostPort plugs in a port of the driver, the IOMMU, the MMU, or the
	// CPU memory.
	BindHostPort(port messaging.Port)

	// BindGPUPort plugs in an external port of the GPU with the given
	// 1-based index.
	BindGPUPort(index int, port messaging.Port)
}

// directNetwork connects all the devices with one ideal direct connection.
type directNetwork struct {
	conn *direct.Connection
}

func (n directNetwork) BindHostPort(port messaging.Port) { n.conn.BindPort(port) }

func (n directNetwork) BindGPUPort(_ int, port messaging.Port) { n.conn.BindPort(port) }

// A wafer-scale GPU connects its host and tiles with waferscale.Network.
var _ interDeviceNetwork = waferscale.Network{}

func (b *Builder) createInterDeviceNetwork() interDeviceNetwork {
	if _, ok := waferscale.ParseGPUType(b.gpuType); ok {
		return waferscale.NewNetwork(b.simulation, b.waferMesh, b.switchLatency)
	}

	return directNetwork{
		conn: direct.NewConnection("InterDeviceConn", b.simulation, timing.GHz),
	}
}
