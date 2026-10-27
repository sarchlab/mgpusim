package dispatching

import "github.com/sarchlab/mgpusim/v5/amd/kernels"

// stateGrid resolves the canonical grid on every operation, including after restore.
type stateGrid struct {
	ids   interface{ NewID() uint64 }
	state func() *kernels.GridBuilderState
}

func (g *stateGrid) SetKernel(info kernels.KernelLaunchInfo) {
	b := kernels.NewGridBuilder(g.ids)
	b.SetKernel(info)
	*g.state() = b.Snapshot()
}
func (g *stateGrid) NumWG() int                 { return g.state().NumWG }
func (g *stateGrid) NextWG() *kernels.WorkGroup { return kernels.NextWorkGroup(g.state(), g.ids) }
func (g *stateGrid) Skip(n int) {
	for i := 0; i < n; i++ {
		g.NextWG()
	}
}
func (g *stateGrid) Snapshot() kernels.GridBuilderState { return *g.state() }
func (g *stateGrid) Restore(s kernels.GridBuilderState) { *g.state() = s }
