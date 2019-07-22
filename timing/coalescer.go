package timing

import "gitlab.com/akita/gcn3/timing/wavefront"

// Coalescer can generate memory access instructions from instruction, register
// values.
type coalescer interface {
	generateMemTransactions(wf *wavefront.Wavefront) []VectorMemAccessInfo
}
