package timing

import (
	"gitlab.com/yaotsu/core"
)

// An GPU is a GPU device that can run kernels. GPU
// ComputeUnits. It contains caches but does not contain GPU memory
type GPU struct {
	*core.BasicComponent

	HWSes []*HWS
	ACEs  []*ACE
	CUs   []*ComputeUnit

	// TODO Caches and Networks
}
