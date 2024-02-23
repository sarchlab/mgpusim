package component

type Benchmark struct {
	tracePath string

	kernelsCount int64
	kernels      []Kernel
}

type Kernel struct {
	threadblocksCount int64
	threadblocks      []Threadblock
}

type Threadblock struct {
	warpsCount int64
	warps      []Warp
}

type Warp struct {
	instructionsCount int64
	instructions      []Instruction
}

type Instruction struct {
}

func BuildBenchmarkFromTrace(tracePath string) *Benchmark {
	// todo
	return &Benchmark{
		tracePath: tracePath,
	}
}

func (b *Benchmark) KernelsCount() int64 {
	return b.kernelsCount
}

func (b *Benchmark) Kernel(id int64) *Kernel {
	return &b.kernels[id]
}

func (k *Kernel) ThreadblocksCount() int64 {
	return k.threadblocksCount
}

func (k *Kernel) Threadblock(id int64) *Threadblock {
	return &k.threadblocks[id]
}

func (tb *Threadblock) WarpsCount() int64 {
	return tb.warpsCount
}

func (tb *Threadblock) Warp(id int64) *Warp {
	return &tb.warps[id]
}

func (w *Warp) InstructionsCount() int64 {
	return w.instructionsCount
}
