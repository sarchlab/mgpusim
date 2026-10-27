package memorybandwidth

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
	Gpus      []int
	Arch      arch.Type
	// NumElements is the number of float32 elements copied device-to-device.
	NumElements      int
	Src              []float32
	GSrc             gpuruntime.Ptr
	GDst             gpuruntime.Ptr
	UseUnifiedMemory bool
}

func (b *Benchmark) bind() error {
	b.loadProgram()
	return nil
}
