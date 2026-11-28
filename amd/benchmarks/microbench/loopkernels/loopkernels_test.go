package loopkernels

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"math"
	"strings"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
)

// kernelSymbols lists the kernels of an embedded code object: its function
// symbols.
func kernelSymbols(object string) []string {
	file, err := elf.NewFile(bytes.NewReader(codeObjects[object]))
	Expect(err).NotTo(HaveOccurred())
	symbols, err := file.Symbols()
	Expect(err).NotTo(HaveOccurred())
	var kernels []string
	for _, symbol := range symbols {
		if elf.ST_TYPE(symbol.Info) == elf.STT_FUNC {
			kernels = append(kernels, symbol.Name)
		}
	}
	return kernels
}

// everyKernel selects the kernel of every parameter combination of both
// benchmarks, keyed by kernel name.
func everyKernel() map[string]Kernel {
	kernels := make(map[string]Kernel)
	for _, instruction := range InstructionCostInstructions() {
		for _, dependency := range InstructionCostDependencies {
			for _, k := range InstructionCostOpsPerIter {
				kernel, err := InstructionCostKernel(instruction, dependency, k, 64)
				Expect(err).NotTo(HaveOccurred())
				kernels[kernel.Name] = kernel
			}
		}
	}
	for _, layout := range LoopWaitLayouts {
		for _, instruction := range VALUInstructions {
			for _, k := range LoopWaitOpsPerIter {
				kernel, err := LoopWaitKernel(layout, instruction, k, 64)
				Expect(err).NotTo(HaveOccurred())
				kernels[kernel.Name] = kernel
			}
		}
	}
	return kernels
}

var _ = Describe("Reference", func() {
	// Values printed by lk_reference of gpu_benchmarks
	// common_hip/gfx942_loop_host.h.
	DescribeTable("matches the HIP host reference",
		func(instruction string, applications uint64, want uint64) {
			Expect(Reference(instruction, applications)).To(Equal(want))
		},
		Entry(nil, "v_fma_f32", uint64(0), uint64(0x3f800000)),
		Entry(nil, "v_fma_f32", uint64(1024), uint64(0x3f800800)),
		Entry(nil, "v_fma_f32", uint64(1<<20), uint64(0x3fa00000)),
		Entry(nil, "v_mul_f32", uint64(1<<20), uint64(0x3f900000)),
		Entry(nil, "v_add_f32", uint64(1024), uint64(0x3f800400)),
		Entry(nil, "v_mul_lo_u32", uint64(2), uint64(0x17385ca9)),
		Entry(nil, "v_mul_lo_u32", uint64(1<<20), uint64(0xfc400001)),
		Entry(nil, "v_add_u32", uint64(1024), uint64(0xbbcd7c01)),
		Entry(nil, "v_fma_f64", uint64(1), uint64(0x3ff0000035afe536)),
		Entry(nil, "v_fma_f64", uint64(1<<20), uint64(0x3ff389a54aff928f)),
		Entry(nil, "v_pk_fma_f32", uint64(0), uint64(0x3f0000003f800000)),
		Entry(nil, "v_pk_fma_f32", uint64(1<<20), uint64(0x3f3000003fa00000)),
		Entry(nil, "v_mov_b64", uint64(1024), uint64(0x0123456789abcdef)),
	)
})

var _ = Describe("Kernel selection", func() {
	It("reaches every embedded kernel, from the code object that holds it", func() {
		kernels := everyKernel()

		total := 0
		for object := range codeObjects {
			for _, name := range kernelSymbols(object) {
				Expect(kernels).To(HaveKey(name))
				Expect(kernels[name].Object).To(Equal(object), name)
				total++
			}
		}
		Expect(kernels).To(HaveLen(total))
		Expect(total).To(Equal(404))
	})

	It("selects ic_empty for zero copies of any body", func() {
		for _, instruction := range []string{"v_fma_f64", "s_branch", "v_mov_b64_v_fma_f64"} {
			kernel, err := InstructionCostKernel(instruction, "independent", 0, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(kernel).To(Equal(Kernel{"ic_empty", InstructionCostObject, 0x0123456789abcdef}))
		}
	})

	It("selects one kernel for both dependencies of a scalar body", func() {
		chained, err := InstructionCostKernel("s_add_u32", "chained", 8, 100)
		Expect(err).NotTo(HaveOccurred())
		independent, err := InstructionCostKernel("s_add_u32", "independent", 8, 100)
		Expect(err).NotTo(HaveOccurred())
		Expect(chained).To(Equal(independent))
		Expect(chained).To(Equal(Kernel{"ic_s_add_u32_k8", InstructionCostOtherObject, 800}))
	})

	It("expects the 32-bit counter of s_add_u32 to wrap", func() {
		kernel, err := InstructionCostKernel("s_add_u32", "chained", 16, math.MaxInt32)
		Expect(err).NotTo(HaveOccurred())
		// 16 * (2^31 - 1) = 2^35 - 16, which is 2^32 - 16 modulo 2^32.
		Expect(kernel.Expected).To(Equal(uint64(math.MaxUint32 - 15)))
	})

	DescribeTable("computes the result of a body",
		func(instruction, dependency string, k int, want uint64) {
			kernel, err := InstructionCostKernel(instruction, dependency, k, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(kernel.Expected).To(Equal(want))
		},
		Entry("chained VALU: every application",
			"v_fma_f32", "chained", 16, Reference("v_fma_f32", 1024)),
		Entry("independent VALU: one application",
			"v_fma_f64", "independent", 16, Reference("v_fma_f64", 1)),
		Entry("chained multiply-add pair (value from the HIP host)",
			"v_mul_lo_u32_v_add_u32", "chained", 16, uint64(0xa938d201)),
		Entry("a multiply-add pair of one copy is a multiply",
			"v_mul_lo_u32_v_add_u32", "chained", 1, Reference("v_mul_lo_u32", 64)),
		Entry("independent multiply-add pair: one multiply",
			"v_mul_lo_u32_v_add_u32", "independent", 16, Reference("v_mul_lo_u32", 1)),
		Entry("chained move-FMA pair: the FMAs only",
			"v_mov_b64_v_fma_f64", "chained", 4, Reference("v_fma_f64", 128)),
		Entry("move-FMA pair of one copy: the initial X",
			"v_mov_b64_v_fma_f64", "chained", 1, Reference("v_fma_f64", 0)),
		Entry("independent move-FMA pair: the initial X",
			"v_mov_b64_v_fma_f64", "independent", 16, Reference("v_fma_f64", 0)),
		Entry("a branch leaves X alone",
			"s_cbranch_scc0", "chained", 16, uint64(0x0123456789abcdef)),
	)

	It("expects a loop_wait kernel to apply every copy", func() {
		kernel, err := LoopWaitKernel("sgpr_in_middle", "v_pk_fma_f32", 16, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(kernel).To(Equal(Kernel{
			"lw_sgpr_in_middle_v_pk_fma_f32_k16", LoopWaitObject, Reference("v_pk_fma_f32", 1024),
		}))
	})

	DescribeTable("rejects parameters that have no kernel",
		func(selectKernel func() (Kernel, error), rejected string) {
			_, err := selectKernel()
			Expect(err).To(MatchError(ContainSubstring(rejected)))
		},
		Entry("unknown instruction", func() (Kernel, error) {
			return InstructionCostKernel("v_div_f32", "chained", 4, 1)
		}, `no instruction "v_div_f32"`),
		Entry("unknown dependency", func() (Kernel, error) {
			return InstructionCostKernel("v_fma_f32", "loose", 4, 1)
		}, `no dependency "loose"`),
		Entry("instruction_cost body size", func() (Kernel, error) {
			return InstructionCostKernel("v_fma_f32", "chained", 3, 1)
		}, "ops_per_iter 3"),
		Entry("unknown layout", func() (Kernel, error) {
			return LoopWaitKernel("head_offset_4", "v_fma_f32", 4, 1)
		}, `no layout "head_offset_4"`),
		Entry("a loop_wait body is a VALU instruction", func() (Kernel, error) {
			return LoopWaitKernel("compiled", "s_add_u32", 4, 1)
		}, `no instruction "s_add_u32"`),
		Entry("loop_wait has no single-copy body", func() (Kernel, error) {
			return LoopWaitKernel("compiled", "v_fma_f32", 1, 1)
		}, "ops_per_iter 1"),
	)

	DescribeTable("checks the launch",
		func(iterations, blocks, threads int, rejected string) {
			err := CheckLaunch(iterations, blocks, threads)
			if rejected == "" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(ContainSubstring(rejected)))
			}
		},
		Entry("default launch", 65536, 1, 64, ""),
		Entry("largest launch", math.MaxInt32, 4194303, 1024, ""),
		Entry("no iterations", 0, 1, 64, "iterations"),
		Entry("iterations beyond int32", math.MaxInt32+1, 1, 64, "iterations"),
		Entry("no threads", 1, 1, 0, "threads per block"),
		Entry("work-group above 1024", 1, 1, 1025, "threads per block"),
		Entry("no blocks", 1, 0, 64, "1 block or more"),
		Entry("grid beyond uint32", 1, 4194304, 1024, "1 block or more"),
	)
})

var _ = Describe("KernelArgs", func() {
	It("is the 272-byte segment hipcc lays out", func() {
		var args KernelArgs
		Expect(binary.Size(args)).To(Equal(272))
		Expect(unsafe.Offsetof(args.Iterations)).To(Equal(uintptr(8)))
		Expect(unsafe.Offsetof(args.Launch)).To(Equal(uintptr(16)))
		// The kernels read hidden_group_size_x at 0x1c.
		Expect(unsafe.Offsetof(args.Launch) +
			unsafe.Offsetof(args.Launch.GroupSize)).To(Equal(uintptr(0x1c)))
		Expect(unsafe.Offsetof(args.GlobalOffset)).To(Equal(uintptr(56)))
		Expect(unsafe.Offsetof(args.GridDims)).To(Equal(uintptr(80)))
	})
})

var _ = Describe("Checkpoint", func() {
	newBenchmark := func(state State) *Benchmark {
		b := &Benchmark{State: &state}
		b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
		return b
	}
	save := func(b *Benchmark) *bytes.Buffer {
		var checkpoint bytes.Buffer
		Expect(b.SaveCheckpoint(&checkpoint)).To(Succeed())
		return &checkpoint
	}
	launch := State{Iterations: 64, NumBlocks: 1, ThreadsPerBlock: 64}

	It("restores the kernel of the saved parameters", func() {
		saved := launch
		saved.Benchmark, saved.Layout, saved.Instruction, saved.OpsPerIter =
			LoopWait, "compiled", "v_fma_f64", 8

		b := newBenchmark(State{Benchmark: InstructionCost})
		Expect(b.LoadCheckpoint(save(newBenchmark(saved)))).To(Succeed())

		Expect(b.hsaco).NotTo(BeNil())
		Expect(b.kernel).To(Equal(Kernel{
			"lw_compiled_v_fma_f64_k8", LoopWaitObject, Reference("v_fma_f64", 512),
		}))
	})

	It("reports parameters that have no kernel and keeps the benchmark", func() {
		saved := launch
		saved.Benchmark, saved.Dependency, saved.Instruction, saved.OpsPerIter =
			InstructionCost, "chained", "v_fma_f32", 3

		current := launch
		current.Benchmark, current.Dependency, current.Instruction, current.OpsPerIter =
			InstructionCost, "independent", "s_branch", 2
		b := newBenchmark(current)
		Expect(b.bind()).To(Succeed())
		kernel := b.hsaco

		err := b.LoadCheckpoint(save(newBenchmark(saved)))
		Expect(err).To(MatchError(ContainSubstring("ops_per_iter 3")))
		Expect(b.OpsPerIter).To(Equal(2))
		Expect(b.hsaco).To(BeIdenticalTo(kernel))
		Expect(b.kernel.Name).To(Equal("ic_s_branch_k2"))
		Expect(b.Step()).To(MatchError(ContainSubstring("ops_per_iter 3")))
	})
})

var _ = Describe("Benchmark", func() {
	It("rejects an unknown benchmark", func() {
		b := &Benchmark{State: &State{
			Benchmark: "loop_cost", Iterations: 1, NumBlocks: 1, ThreadsPerBlock: 64,
		}}
		_, err := b.SelectKernel()
		Expect(err).To(MatchError(ContainSubstring(`unknown loop benchmark "loop_cost"`)))
	})

	It("names every value of its flags in the gpu_benchmarks order", func() {
		Expect(strings.Join(InstructionCostInstructions(), ",")).To(Equal(
			"v_fma_f32,v_mul_f32,v_add_f32,v_mul_lo_u32,v_add_u32,v_fma_f64,v_pk_fma_f32," +
				"v_mov_b64,v_mul_lo_u32_v_add_u32,v_mov_b64_v_fma_f64,s_add_u32,s_cbranch_scc0,s_branch"))
	})
})
