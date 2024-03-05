package benchmark

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	tracereader "github.com/sarchlab/mgpusim/v3/accelsim_tracing/traceReader"
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

	reader := new(tracereader.TraceReaderBuilder).
		WithTraceDirectory(b.traceDirectory).
		Build()
	execMetas := reader.GetExecMetas()

	traceExecs := make([]TraceExec, 0)

	execCount := len(execMetas)
	for i := 0; i < execCount; i++ {
		meta := execMetas[i]
		if meta.ExecType() == nvidia.ExecKernel {
			kernel := b.generateKernelTrace(meta)
			traceExecs = append(traceExecs, kernel)
		} else if meta.ExecType() == nvidia.ExecMemcpy {
			memcpy := b.generateMemcpyTrace(meta)
			traceExecs = append(traceExecs, memcpy)
		}
	}

	return &Benchmark{
		TraceExecs: traceExecs,
	}
}

func (b *BenchmarkBuilder) generateKernelTrace(meta tracereader.TraceExecMeta) *ExecKernel {
	kernelTrace := tracereader.ReadTrace(meta)
	kernel := nvidia.Kernel{}

	kernel.ThreadblocksCount = kernelTrace.ThreadblocksCount()
	for i := int64(0); i < kernel.ThreadblocksCount; i++ {
		tb := nvidia.Threadblock{}
		tb.WarpsCount = kernelTrace.Threadblock(i).WarpsCount()
		for j := int64(0); j < tb.WarpsCount; j++ {
			warp := nvidia.Warp{}
			warp.InstructionsCount = kernelTrace.Threadblock(i).Warp(j).InstructionsCount()
			tb.Warps = append(tb.Warps, warp)
		}
		kernel.Threadblocks = append(kernel.Threadblocks, tb)
	}

	return &ExecKernel{
		kernel: kernel,
	}
}

func (b *BenchmarkBuilder) generateMemcpyTrace(meta tracereader.TraceExecMeta) *ExecMemcpy {
	return &ExecMemcpy{
		direction: meta.Direction,
		address:   meta.Address,
		length:    meta.Length,
	}
}

func (b *BenchmarkBuilder) traceDirectoryMustBeSet() {
	if b.traceDirectory == "" {
		panic("Trace directory must be set")
	}
}
