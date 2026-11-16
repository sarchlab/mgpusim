package cu

// Coalescer can generate memory access instructions from instruction, register
// values.
type coalescer interface {
	generateMemTransactions(wf *Wavefront) []VectorMemAccessInfo
}
