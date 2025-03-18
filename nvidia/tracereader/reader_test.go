package tracereader_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
	"github.com/sarchlab/mgpusim/v4/nvidia/tracereader"
)

var _ = Describe("Read Traces from VectorAdd Traces Version 5.0", func() {
	var traceDir = "../data/simple-trace-example"
	var trace tracereader.KernelTrace
	var once sync.Once

	BeforeEach(func() {
		once.Do(func() {
			tracerder := new(tracereader.TraceReaderBuilder).
				WithTraceDirectory(traceDir).
				Build()
			execs := tracerder.GetExecMetas()
			for _, exec := range execs {
				if exec.ExecType() == nvidiaconfig.ExecKernel {
					trace = tracereader.ReadTrace(exec)
				}
			}
		})
	})

	Describe("Header Extract", func() {
		It("AccelSim Version should be 5", func() {
			Expect(trace.FileHeader.AccelsimTracerVersion).To(Equal("5"))
		})
	})

	Describe("Blocks Count", func() {
		It("should contain 196 block", func() {
			Expect(trace.ThreadblocksCount()).To(Equal(uint64(196)))
		})
	})

	Describe("Insts Count", func() {
		It("should count 26601 instructions", func() {
			instCount := 0
			for i := uint64(0); i < trace.ThreadblocksCount(); i++ {
				tb := trace.Threadblock(i)
				for j := uint64(0); j < tb.WarpsCount(); j++ {
					instCount += int(tb.Warp(j).InstructionsCount())
				}
			}
			Expect(instCount).To(Equal(26601))
		})
	})

})
