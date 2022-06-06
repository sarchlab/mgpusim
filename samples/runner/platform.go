package runner

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
	"gitlab.com/akita/mgpusim/v3/driver"
	"gitlab.com/akita/mgpusim/v3/timing/cp"
	"gitlab.com/akita/mgpusim/v3/timing/pagemigrationcontroller"
	"gitlab.com/akita/mgpusim/v3/timing/rdma"
)

// TraceableComponent is a component that can accept traces
type TraceableComponent interface {
	//sim.Component
	tracing.NamedHookable
}

// A Platform is a collection of the hardware under simulation.
type Platform struct {
	Engine sim.Engine
	Driver *driver.Driver
	GPUs   []*GPU
}

// A GPU is a collection of GPU internal Components
type GPU struct {
	Domain           *sim.Domain
	CommandProcessor *cp.CommandProcessor
	RDMAEngine       *rdma.Engine
	PMC              *pagemigrationcontroller.PageMigrationController
	CUs              []TraceableComponent
	SIMDs            []TraceableComponent
	L1VCaches        []TraceableComponent
	L1SCaches        []TraceableComponent
	L1ICaches        []TraceableComponent
	L2Caches         []TraceableComponent
	L1VTLBs          []TraceableComponent
	L1STLBs          []TraceableComponent
	L1ITLBs          []TraceableComponent
	L2TLBs           []TraceableComponent
	MemControllers   []TraceableComponent
}
