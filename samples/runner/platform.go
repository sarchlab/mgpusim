package runner

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v3/driver"
	"github.com/sarchlab/mgpusim/v3/timing/cp"
	"github.com/sarchlab/mgpusim/v3/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v3/timing/rdma"
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
	RDMAEngine       *rdma.Comp
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
