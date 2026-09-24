package trace_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

var _ = Describe("Trace reader", func() {
	const traceDir = "testdata/vectoradd"

	var (
		metas  []trace.TraceExecMeta
		kernel trace.KernelTrace
	)

	BeforeEach(func() {
		metas = new(trace.TraceReaderBuilder).
			WithTraceDirectory(traceDir).
			Build().
			GetExecMetas()

		for _, m := range metas {
			if m.ExecType() == trace.ExecKernel {
				kernel = trace.ReadTrace(m)
			}
		}
	})

	It("should read kernelslist.g in order", func() {
		Expect(metas).To(HaveLen(3))
		Expect(metas[0].ExecType()).To(Equal(trace.ExecMemcpy))
		Expect(metas[0].Direction).To(Equal(trace.H2D))
		Expect(metas[0].Address).To(Equal(uint64(0x00007fb0fc400000)))
		Expect(metas[0].Length).To(Equal(uint64(200000)))
		Expect(metas[2].ExecType()).To(Equal(trace.ExecKernel))
	})

	It("should parse the kernel header", func() {
		Expect(kernel.FileHeader.KernelName).To(Equal("_Z9vectorAddPKfS0_Pfi"))
		Expect(kernel.FileHeader.AccelsimTracerVersion).To(Equal("5"))
		Expect(kernel.FileHeader.GridDim).To(Equal(trace.Dim3{196, 1, 1}))
		Expect(kernel.FileHeader.BlockDim).To(Equal(trace.Dim3{256, 1, 1}))
	})

	It("should parse thread blocks, warps, and instructions", func() {
		Expect(kernel.ThreadblocksCount()).To(Equal(uint64(2)))
		Expect(kernel.Threadblock(1).ID).To(Equal(trace.Dim3{1, 0, 0}))
		Expect(kernel.Threadblock(0).WarpsCount()).To(Equal(uint64(8)))

		instCount := uint64(0)
		for _, tb := range kernel.Threadblocks {
			for _, w := range tb.Warps {
				instCount += w.InstructionsCount()
			}
		}
		Expect(instCount).To(Equal(uint64(272)))

		ldg := kernel.Threadblock(0).Warp(0).Instructions[10]
		Expect(ldg.OpCode.String()).To(Equal("LDG.E"))
		Expect(ldg.MemWidth).To(Equal(4))
		Expect(ldg.MemAddress).To(Equal(uint64(0x7fb0fc430e00)))
		Expect(ldg.SrcRegs).To(Equal([]trace.Register{{Name: "R4"}}))
		Expect(ldg.DestRegs).To(Equal([]trace.Register{{Name: "R4"}}))
	})
})

var _ = Describe("Trace reader with tracer version 6", func() {
	It("should parse a trace that carries register value columns", func() {
		metas := new(trace.TraceReaderBuilder).
			WithTraceDirectory("testdata/atax-v6").
			Build().
			GetExecMetas()
		Expect(metas).To(HaveLen(3))

		kernel := trace.ReadTrace(metas[2])
		Expect(kernel.FileHeader.AccelsimTracerVersion).To(Equal("6"))
		Expect(kernel.FileHeader.KernelName).To(Equal("atax_kernel1"))
		Expect(kernel.ThreadblocksCount()).To(Equal(uint64(1)))
		Expect(kernel.Threadblock(0).WarpsCount()).To(Equal(uint64(2)))

		warp := kernel.Threadblock(0).Warp(1)
		Expect(warp.InstructionsCount()).To(Equal(uint64(30)))

		var ldg *trace.InstructionTrace
		for _, inst := range warp.Instructions {
			if inst.OpCode == "LDG.E" {
				ldg = inst
				break
			}
		}
		Expect(ldg).NotTo(BeNil())
		Expect(ldg.MemAddress).To(Equal(uint64(0x7f7b7dc40880)))
		Expect(ldg.MemAddressSuffix1).To(Equal(4))
	})
})
