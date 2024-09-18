package tracereader_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/tracereader"
)

var _ = Describe("Read Traces from VectorAdd Traces Version 5.0", func() {
	var traceDir = "../data/cuda-samples/0_Introduction/vectorAdd"
	var trace tracereader.KernelTrace
	var once sync.Once

	BeforeEach(func() {
		once.Do(func() {
			tracerder := new(tracereader.TraceReaderBuilder).
				WithTraceDirectory(traceDir).
				Build()
			execs := tracerder.GetExecMetas()
			for _, exec := range execs {
				if exec.ExecType() == nvidia.ExecKernel {
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
			Expect(trace.ThreadblocksCount()).To(Equal(int64(196)))
		})
	})

	Describe("Insts Count", func() {
		It("should count 26601 instructions", func() {
			instCount := 0
			for i := int64(0); i < trace.ThreadblocksCount(); i++ {
				tb := trace.Threadblock(i)
				for j := int64(0); j < tb.WarpsCount(); j++ {
					instCount += int(tb.Warp(j).InstructionsCount())
				}
			}
			Expect(instCount).To(Equal(26601))
		})
	})

})
