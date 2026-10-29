package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/pagemigration"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// Port buffer sizes of page migration.
const (
	pmCtrlPortBufSize   = 64
	pmDriverPortBufSize = 64
	pmMoverMemBufSize   = 256
)

// PageMigrationConfig is what a GPU builder needs to know about page
// migration. A nil config disables page migration.
type PageMigrationConfig struct {
	Spec pagemigration.Spec

	// DriverPort is the page-migration port of the driver, where the access
	// counters send their notifications.
	DriverPort messaging.RemotePort
}

// VectorATsCanFault reports whether the address translators of the vector
// memory accesses may fault.
func (c *PageMigrationConfig) VectorATsCanFault() bool {
	return c != nil && c.Spec.Policy == pagemigration.PolicyOnFault
}

// ConfigureAccessCounter sets the access counter of the RDMA engine of GPU
// gpuID: it counts for the statistics whenever page migration is enabled,
// and notifies the driver with the access-counter policy.
func (c *PageMigrationConfig) ConfigureAccessCounter(
	spec *rdma.Spec,
	gpuID uint64,
) {
	if c == nil || !c.Spec.Enabled() {
		return
	}

	spec.CountAccesses = true
	spec.DeviceID = gpuID
	spec.DeviceMemSize = c.Spec.GPUMemSize
	spec.CounterLog2PageSize = c.Spec.Log2PageSize

	if c.Spec.Policy == pagemigration.PolicyAccessCounter {
		spec.CounterThreshold = c.Spec.Threshold
		spec.CounterNotifyDst = c.DriverPort
	}
}

// Connection is a connection that ports can be plugged into.
type Connection interface {
	PlugIn(port messaging.Port)
}

// PageMigrationParts are the parts of a GPU that page migration wires to
// after every GPU is built.
type PageMigrationParts struct {
	CP *cp.Comp

	// InternalConn connects the command processor to the Control ports of
	// the address translators and the TLBs.
	InternalConn Connection

	// MemConn and MemMapper reach the memory-side modules that the data
	// mover writes the new frames to, around the L2.
	MemConn   Connection
	MemMapper *mem.InterleavedAddressPortMapper
}

// BuildPageMigration builds the data mover of every GPU and installs the
// page-migration middleware in every command processor. gpus are the GPUs
// with IDs 1, 2, .... It returns the ports to plug into the inter-device
// connection.
func BuildPageMigration(
	reg modeling.Registrar,
	spec pagemigration.Spec,
	gpus []*GPU,
) []messaging.Port {
	if !spec.Migrates() {
		return nil
	}

	// The data mover of a destination reads the source page through the
	// RDMA engine of the source GPU; device d owns the d-th bank.
	remote := mem.NewInterleavedAddressPortMapper(spec.GPUMemSize)
	remote.LowModules = append(remote.LowModules, messaging.RemotePort("CPU"))

	for _, g := range gpus {
		remote.LowModules = append(remote.LowModules, g.RDMADataPort.AsRemote())
	}

	var external []messaging.Port

	for _, g := range gpus {
		external = append(external, buildGPUPageMigration(reg, spec, g, remote)...)
	}

	return external
}

func buildGPUPageMigration(
	reg modeling.Registrar,
	spec pagemigration.Spec,
	g *GPU,
	remote *mem.InterleavedAddressPortMapper,
) []messaging.Port {
	parts := g.PageMigration

	moverSpec := datamover.DefaultSpec()
	moverSpec.Freq = spec.Freq
	moverSpec.BufferSize = spec.CopyBufferBytes
	moverSpec.InsideByteGranularity = spec.LineBytes
	moverSpec.OutsideByteGranularity = spec.LineBytes

	mover := datamover.MakeBuilder().
		WithRegistrar(reg).
		WithSpec(moverSpec).
		WithResources(datamover.Resources{
			InsideMapper:  parts.MemMapper,
			OutsideMapper: remote,
		}).
		Build(g.Name + ".PageMigrationDataMover")

	buildPort(reg, mover, "Top", pmCtrlPortBufSize)
	buildPort(reg, mover, "Control", pmCtrlPortBufSize)
	buildPort(reg, mover, "Inside", pmMoverMemBufSize)
	buildPort(reg, mover, "Outside", pmMoverMemBufSize)

	parts.InternalConn.PlugIn(mover.GetPortByName("Top"))
	parts.InternalConn.PlugIn(mover.GetPortByName("Control"))
	parts.MemConn.PlugIn(mover.GetPortByName("Inside"))

	cpComp := parts.CP
	pmState := &cpComp.State.PageMigration
	pmState.AddressTranslators = append(pmState.AddressTranslators[:0],
		cpComp.State.AddressTranslators...)
	pmState.TLBs = append(pmState.TLBs[:0], cpComp.State.TLBs...)
	pmState.DataMover = mover.GetPortByName("Top").AsRemote()

	cpComp.AddMiddleware(pagemigration.NewCPMiddleware(cpComp, pmState))
	cpComp.DeclarePort(pagemigration.CPDriverPort)
	cpComp.DeclarePort(pagemigration.CPCtrlPort,
		memcontrolprotocol.Requester, datamoverprotocol.Requester)

	buildPort(reg, cpComp, pagemigration.CPDriverPort, pmDriverPortBufSize)
	buildPort(reg, cpComp, pagemigration.CPCtrlPort, pmCtrlPortBufSize)
	parts.InternalConn.PlugIn(cpComp.GetPortByName(pagemigration.CPCtrlPort))

	return []messaging.Port{
		cpComp.GetPortByName(pagemigration.CPDriverPort),
		mover.GetPortByName("Outside"),
	}
}

func buildPort(
	reg modeling.Registrar,
	comp messaging.Component,
	name string,
	bufSize int,
) {
	port := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, port)
}
