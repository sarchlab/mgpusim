package cu

import "github.com/sarchlab/mgpusim/v3/timing/wavefront"

// Coalescer can generate memory access instructions from instruction, register
// values.
type coalescer interface {
	generateMemTransactions(wf *wavefront.Wavefront) []VectorMemAccessInfo
}
