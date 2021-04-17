package runner

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/pagemigrationcontroller"
	"gitlab.com/akita/mgpusim/v2/rdma"
	"gitlab.com/akita/mgpusim/v2/timing/cp"
	"gitlab.com/akita/util/v2/tracing"
)

// TraceableComponent is a component that can accept traces
type TraceableComponent interface {
	sim.Component
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
	L1VCaches        []TraceableComponent
	L1SCaches        []TraceableComponent
	L1ICaches        []TraceableComponent
	L2Caches         []TraceableComponent
	MemControllers   []TraceableComponent
}
