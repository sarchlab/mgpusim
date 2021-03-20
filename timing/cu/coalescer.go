package cu

import "gitlab.com/akita/mgpusim/v2/timing/wavefront"

// Coalescer can generate memory access instructions from instruction, register
// values.
type coalescer interface {
	generateMemTransactions(wf *wavefront.Wavefront) []VectorMemAccessInfo
}
