package resource

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/kernels"
)

// CUResource handle CU resources
type CUResource interface {
	ReserveResourceForWG(wg *kernels.WorkGroup) (
		locations []WfLocation,
		ok bool,
	)
	FreeResourcesForWG(wg *kernels.WorkGroup)
	DispatchingPort() sim.Port
}
