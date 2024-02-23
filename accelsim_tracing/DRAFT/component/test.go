package component

/* kernel(10) -> threadblock(2) -> warp(4) -> instruction(100) */

func NewBenchmarkForTest() *Benchmark {
	kernelsCount := 10

	bc := &Benchmark{
		kernelsCount: int64(kernelsCount),
		kernels:      make([]Kernel, kernelsCount),
	}

	for i := int64(0); i < bc.kernelsCount; i++ {
		bc.kernels[i] = NewKernelForTest()
	}

	return bc
}

func NewKernelForTest() Kernel {
	threadblocksCount := 2

	k := Kernel{
		threadblocksCount: int64(threadblocksCount),
		threadblocks:      make([]Threadblock, 2),
	}

	for i := int64(0); i < k.threadblocksCount; i++ {
		k.threadblocks[i] = NewThreadblockForTest()
	}

	return k
}

func NewThreadblockForTest() Threadblock {
	warpsCount := 4

	tb := Threadblock{
		warpsCount: int64(warpsCount),
		warps:      make([]Warp, 4),
	}

	for i := int64(0); i < tb.warpsCount; i++ {
		tb.warps[i] = NewWarpForTest()
	}

	return tb
}

func NewWarpForTest() Warp {
	instructionsCount := 100

	w := Warp{
		instructionsCount: int64(instructionsCount),
		instructions:      make([]Instruction, 100),
	}

	return w
}
