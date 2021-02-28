package dispatching

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/protocol"
	"gitlab.com/akita/mgpusim/timing/cp/internal/resource"
)

type dispatchLocation struct {
	valid     bool
	cuID      int
	cu        akita.Port
	wg        *kernels.WorkGroup
	locations []protocol.WfDispatchLocation
}

// algorithm defines the CTA scheduling scheme.
type algorithm interface {
	// RegisterCU notifies the algorithm about the existence of the a cu.
	RegisterCU(cu resource.DispatchableCU)

	// StartNewKernel allows the algorithm start to process another kernel.
	StartNewKernel(info kernels.KernelLaunchInfo)

	// NumWG returns the total number of work-groups.
	NumWG() int

	// HasNext checks if there are more work-groups need to dispatch
	HasNext() bool

	// Next returns the information about where the next workgroup can be
	// dispatched.
	Next() (location dispatchLocation)

	// FreeResources marks the dispatched resources available.
	FreeResources(location dispatchLocation)
}
