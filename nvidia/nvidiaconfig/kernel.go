package nvidiaconfig

type Kernel struct {
	ThreadblocksCount uint64
	Threadblocks      []Threadblock
}

type Threadblock struct {
	WarpsCount uint64
	Warps      []Warp
}

type Warp struct {
	InstructionsCount uint64
	Instructions      []Instruction
}

type Instruction struct {
}
