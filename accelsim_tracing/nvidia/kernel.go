package nvidia

type Kernel struct {
	ThreadblocksCount int64
	Threadblocks      []Threadblock
}

type Threadblock struct {
	WarpsCount int64
	Warps      []Warp
}

type Warp struct {
	InstructionsCount int64
	Instructions      []Instruction
}

type Instruction struct {
}
