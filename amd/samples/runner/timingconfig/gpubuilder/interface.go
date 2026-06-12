// Package gpubuilder defines the interface for GPU builders used in timing
// simulation.
package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
)

// GPU is the externally visible handle of a built GPU. It replaces the v4
// sim.Domain: instead of registering external ports on a domain, the GPU
// builders return the port instances that the platform needs to wire the GPU
// to the driver, the MMU, and the other GPUs.
type GPU struct {
	// Name is the name the GPU was built with (e.g., "GPU[1]").
	Name string

	// CommandProcessor is the GPU's command processor. The platform
	// configuration uses it to set the driver destination port
	// (CommandProcessor.State.Driver) after the GPU is built.
	CommandProcessor *cp.Comp

	// CommandProcessorPort is the command processor's driver-facing port
	// (the CP's "ToDriver" port; v4 domain port "CommandProcessor").
	CommandProcessorPort messaging.Port

	// RDMARequestPort is the RDMA engine's outside request port (v4 domain
	// port "RDMARequest").
	RDMARequestPort messaging.Port

	// RDMADataPort is the RDMA engine's outside data port (v4 domain port
	// "RDMAData").
	RDMADataPort messaging.Port

	// TranslationPorts are the bottom ports of the L2 TLBs. They send
	// translation requests to the MMU over the inter-device network (v4
	// domain ports "Translation_xx").
	TranslationPorts []messaging.Port
}

// ExternalPorts returns all the ports of the GPU that connect to the
// inter-device network (the v4 Domain.Ports() equivalent).
func (g *GPU) ExternalPorts() []messaging.Port {
	ports := []messaging.Port{
		g.CommandProcessorPort,
		g.RDMARequestPort,
		g.RDMADataPort,
	}
	ports = append(ports, g.TranslationPorts...)

	return ports
}

// GPUBuilder is the interface for building GPUs of different types.
type GPUBuilder interface {
	WithGPUID(id uint64) GPUBuilder
	WithMemAddrOffset(offset uint64) GPUBuilder
	WithRDMAAddressMapper(mapper mem.AddressToPortMapper) GPUBuilder
	WithDriverPort(port messaging.RemotePort) GPUBuilder
	Build(name string) *GPU
}
