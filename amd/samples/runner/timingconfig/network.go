package timingconfig

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
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

	// Connect finishes the topology after every port is bound.
	Connect()
}

var (
	_ interDeviceNetwork = r9nano.Network{}
	_ interDeviceNetwork = mi300x.Network{}
	_ interDeviceNetwork = waferscale.Network{}
)

func (b *Builder) createInterDeviceNetwork() interDeviceNetwork {
	if _, ok := waferscale.ParseGPUType(b.gpuType); ok {
		return waferscale.NewNetwork(b.simulation, b.waferMesh, b.switchLatency)
	}

	if b.gpuType == "mi300x" {
		return mi300x.NewNetwork(b.simulation, b.numGPUs, b.switchLatency)
	}

	return r9nano.NewNetwork(b.simulation, b.numGPUs, b.switchLatency)
}
