package resource

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
)

// CUResource handle CU resources
type CUResource interface {
	ReserveResourceForWG(wg *kernels.WorkGroup) (
		locations []WfLocation,
		ok bool,
	)
	FreeResourcesForWG(wg *kernels.WorkGroup)
	DispatchingPort() akita.Port
}
