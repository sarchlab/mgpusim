package resource

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// CUResource handle CU resources
type CUResource interface {
	ReserveResourceForWG(wg *kernels.WorkGroup) (
		locations []WfLocation,
		ok bool,
	)
	FreeResourcesForWG(wg *kernels.WorkGroup)
	DispatchingPort() sim.RemotePort
}
