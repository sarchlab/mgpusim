package timing

import (
	"gitlab.com/yaotsu/core"
)

// An GPU is a GPU device that can run kernels. GPU
// ComputeUnits. It contains caches but does not contain GPU memory
type GPU struct {
	*core.ComponentBase

	Dispatcher []*core.Component
	CUs        []*core.Component

	// TODO Caches and Networks
}
