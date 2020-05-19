package resource

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
)

type CUResource interface {
	ReserveResourceForWG(wg *kernels.WorkGroup) (
		locations []WfLocation,
		ok bool,
	)
	FreeResourcesForWG(wg *kernels.WorkGroup)
	DispatchingPort() akita.Port
}
