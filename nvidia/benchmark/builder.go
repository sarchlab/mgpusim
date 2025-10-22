package benchmark

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type BenchmarkBuilder struct {
	traceDirectory string
}

func (b *BenchmarkBuilder) WithTraceDirectory(traceDirectory string) *BenchmarkBuilder {
	b.traceDirectory = traceDirectory
	return b
}

func (b *BenchmarkBuilder) Build() *Benchmark {
	b.traceDirectoryMustBeSet()

	reader := new(trace.TraceReaderBuilder).
		WithTraceDirectory(b.traceDirectory).
		Build()
	execMetas := reader.GetExecMetas()

	traceExecs := make([]TraceExec, 0)

	execCount := len(execMetas)
	for i := 0; i < execCount; i++ {
		meta := execMetas[i]
		// fmt.Println("meta.execType: ", meta.ExecType())
		if meta.ExecType() == trace.ExecKernel {
			kernel := b.generateKernelTrace(meta)
			traceExecs = append(traceExecs, kernel)
		} else if meta.ExecType() == trace.ExecMemcpy {
			memcpy := b.generateMemcpyTrace(meta)
			traceExecs = append(traceExecs, memcpy)
		}
	}
	// for j := 0; j < execCount; j++ {
	// 	fmt.Println("Print TraceExec: ", execMetas[j].ExecType())
	// }

	return &Benchmark{
		TraceExecs: traceExecs,
	}
}

func (b *BenchmarkBuilder) generateKernelTrace(meta trace.TraceExecMeta) *ExecKernel {
	k := trace.ReadTrace(meta)
	// kernel := trace.KernelTrace{}

	// // kernel.ThreadblocksCount = kernelTrace.ThreadblocksCount()
	// fmt.Println("kernel is read")
	// for i := uint64(0); i < kernelRead.ThreadblocksCount(); i++ {
	// 	tb := trace.ThreadblockTrace{}
	// 	// tb.WarpsCount = kernelTrace.Threadblock(i).WarpsCount()
	// 	for j := uint64(0); j < tb.WarpsCount(); j++ {
	// 		warp := trace.WarpTrace{}
	// 		// warp.InstructionsCount = kernelTrace.Threadblock(i).Warp(j).InstructionsCount()
	// 		instruction := trace.InstructionTrace{}
	// 		warp.Instructions = append(warp.Instructions, &instruction)
	// 		tb.Warps = append(tb.Warps, &warp)
	// 	}
	// 	kernel.Threadblocks = append(kernel.Threadblocks, &tb)
	// }
	// fmt.Println("kernel is set")

	return &ExecKernel{
		kernel: k,
	}
}

func (b *BenchmarkBuilder) generateMemcpyTrace(meta trace.TraceExecMeta) *ExecMemcpy {
	return &ExecMemcpy{
		direction: meta.Direction,
		address:   meta.Address,
		length:    meta.Length,
	}
}

func (b *BenchmarkBuilder) traceDirectoryMustBeSet() {
	if b.traceDirectory == "" {
		log.Panic("Trace directory must be set")
	}
}
