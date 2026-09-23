package dispatching

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

type dispatchLocation struct {
	valid     bool
	cuID      int
	cu        messaging.RemotePort
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

// dieAwareAlgorithm dispatches a kernel across multiple dies (XCDs) in parallel.
// The dispatcher drives each die independently through NextForDie and gates each
// die with its own per-wavefront dispatch rate, so the dies advance in parallel.
type dieAwareAlgorithm interface {
	algorithm

	// NumDies returns the number of dies dispatched across in parallel.
	NumDies() int

	// NextForDie returns where the next work-group on the given die can be
	// dispatched, or an invalid location if that die has nothing to dispatch
	// right now.
	NextForDie(die int) (location dispatchLocation)
}
