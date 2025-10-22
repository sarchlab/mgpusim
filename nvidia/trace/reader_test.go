package trace_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

var _ = Describe("Read Traces from VectorAdd Traces Version 5.0", func() {
	var traceDir = "../data/simple-trace-example"
	var kt trace.KernelTrace
	var once sync.Once

	BeforeEach(func() {
		once.Do(func() {
			tracerder := new(trace.TraceReaderBuilder).
				WithTraceDirectory(traceDir).
				Build()
			execs := tracerder.GetExecMetas()
			for _, exec := range execs {
				if exec.ExecType() == trace.ExecKernel {
					kt = trace.ReadTrace(exec)
				}
			}
		})
	})

	Describe("Header Extract", func() {
		It("AccelSim Version should be 5", func() {
			Expect(kt.FileHeader.AccelsimTracerVersion).To(Equal("5"))
		})
	})

	Describe("Blocks Count", func() {
		It("should contain 196 block", func() {
			Expect(kt.ThreadblocksCount()).To(Equal(uint64(196)))
		})
	})

	Describe("Insts Count", func() {
		It("should count 26601 instructions", func() {
			instCount := 0
			for i := uint64(0); i < kt.ThreadblocksCount(); i++ {
				tb := kt.Threadblock(i)
				for j := uint64(0); j < tb.WarpsCount(); j++ {
					instCount += int(tb.Warp(j).InstructionsCount())
				}
			}
			Expect(instCount).To(Equal(26601))
		})
	})

})
