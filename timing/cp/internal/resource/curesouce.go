package resource

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mgpusim/v3/kernels"
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
