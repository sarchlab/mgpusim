package benchmark

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
	"github.com/sarchlab/mgpusim/v4/nvidia/tracereader"
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
		// fmt.Println("meta.execType: ", meta.ExecType())
		if meta.ExecType() == nvidiaconfig.ExecKernel {
			kernel := b.generateKernelTrace(meta)
			traceExecs = append(traceExecs, kernel)
		} else if meta.ExecType() == nvidiaconfig.ExecMemcpy {
			memcpy := b.generateMemcpyTrace(meta)
			traceExecs = append(traceExecs, memcpy)
		}
	}
	for j := 0; j < execCount; j++ {
		fmt.Println("Print TraceExec: ", execMetas[j].ExecType())
	}

	return &Benchmark{
		TraceExecs: traceExecs,
	}
}

func (b *BenchmarkBuilder) generateKernelTrace(meta tracereader.TraceExecMeta) *ExecKernel {
	kernelTrace := tracereader.ReadTrace(meta)
	kernel := nvidiaconfig.Kernel{}

	kernel.ThreadblocksCount = kernelTrace.ThreadblocksCount()
	for i := uint64(0); i < kernel.ThreadblocksCount; i++ {
		tb := nvidiaconfig.Threadblock{}
		tb.WarpsCount = kernelTrace.Threadblock(i).WarpsCount()
		for j := uint64(0); j < tb.WarpsCount; j++ {
			warp := nvidiaconfig.Warp{}
			warp.InstructionsCount = kernelTrace.Threadblock(i).Warp(j).InstructionsCount()
			// instruction := nvidiaconfig.Instruction{}
			// warp.Instructions = append(warp.Instructions)
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
		log.Panic("Trace directory must be set")
	}
}
