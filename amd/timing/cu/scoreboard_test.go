package cu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

var _ = Describe("Scoreboard", func() {
	var sb *Scoreboard

	BeforeEach(func() {
		sb = NewScoreboard()
	})

	Describe("Tick", func() {
		It("should decrement all non-zero counters by 1", func() {
			sb.VGPRBusyUntil[0] = 5
			sb.VGPRBusyUntil[10] = 1
			sb.SGPRBusyUntil[3] = 3
			sb.SCCBusyUntil = 2
			sb.VCCBusyUntil = 4
			sb.EXECBusyUntil = 1

			sb.Tick()

			Expect(sb.VGPRBusyUntil[0]).To(Equal(4))
			Expect(sb.VGPRBusyUntil[10]).To(Equal(0))
			Expect(sb.SGPRBusyUntil[3]).To(Equal(2))
			Expect(sb.SCCBusyUntil).To(Equal(1))
			Expect(sb.VCCBusyUntil).To(Equal(3))
			Expect(sb.EXECBusyUntil).To(Equal(0))
		})

		It("should not decrement counters below zero", func() {
			sb.VGPRBusyUntil[0] = 0
			sb.SCCBusyUntil = 0

			sb.Tick()

			Expect(sb.VGPRBusyUntil[0]).To(Equal(0))
			Expect(sb.SCCBusyUntil).To(Equal(0))
		})
	})

	Describe("MarkBusy", func() {
		It("should mark VGPR destination busy", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.Dst = insts.NewVRegOperand(0, 5, 1)

			sb.MarkBusy(inst, 4)

			Expect(sb.VGPRBusyUntil[5]).To(Equal(4))
		})

		It("should mark multi-register VGPR destination busy", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.Dst = insts.NewVRegOperand(0, 2, 4)

			sb.MarkBusy(inst, 4)

			Expect(sb.VGPRBusyUntil[2]).To(Equal(4))
			Expect(sb.VGPRBusyUntil[3]).To(Equal(4))
			Expect(sb.VGPRBusyUntil[4]).To(Equal(4))
			Expect(sb.VGPRBusyUntil[5]).To(Equal(4))
			Expect(sb.VGPRBusyUntil[6]).To(Equal(0))
		})

		It("should mark SGPR destination busy", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitScalar
			inst.Dst = insts.NewSRegOperand(0, 10, 2)

			sb.MarkBusy(inst, 4)

			Expect(sb.SGPRBusyUntil[10]).To(Equal(4))
			Expect(sb.SGPRBusyUntil[11]).To(Equal(4))
			// Scalar ALU implicitly writes SCC
			Expect(sb.SCCBusyUntil).To(Equal(4))
		})

		It("should mark VCC busy for VOPC instructions", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.FormatType = insts.VOPC
			inst.Src0 = insts.NewVRegOperand(0, 0, 1)
			inst.Src1 = insts.NewVRegOperand(1, 1, 1)

			sb.MarkBusy(inst, 4)

			Expect(sb.VCCBusyUntil).To(Equal(4))
		})

		It("should not mark anything for zero latency", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVMem
			inst.Dst = insts.NewVRegOperand(0, 0, 1)

			sb.MarkBusy(inst, 0)

			Expect(sb.VGPRBusyUntil[0]).To(Equal(0))
		})

		It("should mark SDst busy", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.SDst = insts.NewSRegOperand(0, 0, 1)

			sb.MarkBusy(inst, 4)

			Expect(sb.SGPRBusyUntil[0]).To(Equal(4))
		})
	})

	Describe("HasHazard", func() {
		It("should return true when src reads a busy VGPR", func() {
			sb.VGPRBusyUntil[3] = 10

			inst := insts.NewInst()
			inst.Src0 = insts.NewVRegOperand(0, 3, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should return false when no dependency", func() {
			sb.VGPRBusyUntil[3] = 10

			inst := insts.NewInst()
			inst.Src0 = insts.NewVRegOperand(0, 5, 1)

			Expect(sb.HasHazard(inst)).To(BeFalse())
		})

		It("should check multi-register source operands", func() {
			sb.VGPRBusyUntil[4] = 5

			inst := insts.NewInst()
			inst.Src0 = insts.NewVRegOperand(0, 2, 4)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should check SGPR source operands", func() {
			sb.SGPRBusyUntil[7] = 3

			inst := insts.NewInst()
			inst.Src0 = insts.NewSRegOperand(0, 7, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should check SCC hazard", func() {
			sb.SCCBusyUntil = 2

			inst := insts.NewInst()
			inst.Src0 = insts.NewRegOperand(253, insts.SCC, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should check VCC hazard", func() {
			sb.VCCBusyUntil = 2

			inst := insts.NewInst()
			inst.Src0 = insts.NewRegOperand(106, insts.VCCLO, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should check EXEC hazard", func() {
			sb.EXECBusyUntil = 2

			inst := insts.NewInst()
			inst.Src0 = insts.NewRegOperand(126, insts.EXECLO, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should return false when all counters are zero", func() {
			inst := insts.NewInst()
			inst.Src0 = insts.NewVRegOperand(0, 0, 1)
			inst.Src1 = insts.NewSRegOperand(0, 0, 1)

			Expect(sb.HasHazard(inst)).To(BeFalse())
		})

		It("should check Addr operand", func() {
			sb.VGPRBusyUntil[10] = 3

			inst := insts.NewInst()
			inst.Addr = insts.NewVRegOperand(0, 10, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})

		It("should check Data operand", func() {
			sb.VGPRBusyUntil[20] = 3

			inst := insts.NewInst()
			inst.Data = insts.NewVRegOperand(0, 20, 1)

			Expect(sb.HasHazard(inst)).To(BeTrue())
		})
	})

	Describe("Clear", func() {
		It("should reset all counters to 0", func() {
			sb.VGPRBusyUntil[0] = 10
			sb.VGPRBusyUntil[100] = 5
			sb.SGPRBusyUntil[50] = 3
			sb.SCCBusyUntil = 7
			sb.VCCBusyUntil = 2
			sb.EXECBusyUntil = 1

			sb.Clear()

			Expect(sb.VGPRBusyUntil[0]).To(Equal(0))
			Expect(sb.VGPRBusyUntil[100]).To(Equal(0))
			Expect(sb.SGPRBusyUntil[50]).To(Equal(0))
			Expect(sb.SCCBusyUntil).To(Equal(0))
			Expect(sb.VCCBusyUntil).To(Equal(0))
			Expect(sb.EXECBusyUntil).To(Equal(0))
		})
	})

	Describe("AnyBusy", func() {
		It("should return false when all counters are zero", func() {
			Expect(sb.AnyBusy()).To(BeFalse())
		})

		It("should return true when a VGPR is busy", func() {
			sb.VGPRBusyUntil[5] = 3
			Expect(sb.AnyBusy()).To(BeTrue())
		})

		It("should return true when an SGPR is busy", func() {
			sb.SGPRBusyUntil[2] = 1
			Expect(sb.AnyBusy()).To(BeTrue())
		})

		It("should return true when SCC is busy", func() {
			sb.SCCBusyUntil = 1
			Expect(sb.AnyBusy()).To(BeTrue())
		})

		It("should return true when VCC is busy", func() {
			sb.VCCBusyUntil = 1
			Expect(sb.AnyBusy()).To(BeTrue())
		})

		It("should return true when EXEC is busy", func() {
			sb.EXECBusyUntil = 1
			Expect(sb.AnyBusy()).To(BeTrue())
		})
	})

	Describe("GetScoreboardLatency", func() {
		It("should return 4 for VALU FP32", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.InstName = "v_add_f32"

			Expect(GetScoreboardLatency(inst)).To(Equal(4))
		})

		It("should return 8 for VALU FP64", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVALU
			inst.InstName = "v_add_f64"

			Expect(GetScoreboardLatency(inst)).To(Equal(8))
		})

		DescribeTable("VALU instructions on the double-width datapath",
			func(name string, latency int) {
				inst := insts.NewInst()
				inst.ExeUnit = insts.ExeUnitVALU
				inst.InstName = name

				Expect(usesTwoVGPRsPerLane(name)).To(Equal(latency == 8))
				Expect(GetScoreboardLatency(inst)).To(Equal(latency))
			},
			Entry("f32 FMA", "v_fma_f32", 4),
			Entry("packed f16 FMA", "v_pk_fma_f16", 4),
			Entry("f64 FMA", "v_fma_f64", 8),
			Entry("f64 square root", "v_sqrt_f64", 8),
			Entry("packed f32 FMA", "v_pk_fma_f32", 8),
			Entry("packed f32 multiply", "v_pk_mul_f32", 8),
			Entry("packed f32 add", "v_pk_add_f32", 8),
			Entry("packed b32 move", "v_pk_mov_b32", 8),
			Entry("b64 move", "v_mov_b64_e32", 8),
			Entry("64-bit shift-add stays narrow", "v_lshl_add_u64", 4),
			Entry("64-bit shift stays narrow", "v_lshlrev_b64", 4),
			Entry("64-bit multiply-add stays narrow", "v_mad_u64_u32", 4),
		)

		It("should return 2 for scalar", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitScalar

			Expect(GetScoreboardLatency(inst)).To(Equal(2))
		})

		It("should return 3 for branch", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitBranch

			Expect(GetScoreboardLatency(inst)).To(Equal(3))
		})

		It("should return 0 for VMem", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitVMem

			Expect(GetScoreboardLatency(inst)).To(Equal(0))
		})

		It("should return 0 for LDS", func() {
			inst := insts.NewInst()
			inst.ExeUnit = insts.ExeUnitLDS

			Expect(GetScoreboardLatency(inst)).To(Equal(0))
		})
	})
})

func TestFP64FMACWaitsForAccumulatorPair(t *testing.T) {
	d := insts.NewDisassembler()
	d.IsCDNA3 = true
	inst, err := d.Decode([]byte{0x08, 0x21, 0x00, 0x08})
	if err != nil {
		t.Fatal(err)
	}

	for _, reg := range []int{0, 1} {
		scoreboard := NewScoreboard()
		scoreboard.VGPRBusyUntil[reg] = 1
		if !scoreboard.HasHazard(inst) {
			t.Fatalf("FMAC did not wait for incoming accumulator register v%d", reg)
		}
		scoreboard.Tick()
		if scoreboard.HasHazard(inst) {
			t.Fatalf("FMAC still blocked after v%d became available", reg)
		}
	}
}

func TestCDNA3VOP2WaitsForFP32Accumulator(t *testing.T) {
	cases := []struct {
		name string
		word []byte
	}{
		{"v_mac_f32_e32", []byte{0x03, 0x21, 0x0e, 0x2c}},
		{"v_fmac_f32_e32", []byte{0x03, 0x21, 0x0e, 0x76}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := insts.NewDisassembler()
			scoreboard := NewScoreboard()
			scoreboard.VGPRBusyUntil[7] = 1

			legacy, err := d.Decode(tc.word)
			if err != nil {
				t.Fatal(err)
			}
			if scoreboard.HasHazard(legacy) {
				t.Fatal("legacy decoder changed its hazard behavior")
			}

			d.IsCDNA3 = true
			inst, err := d.Decode(tc.word)
			if err != nil {
				t.Fatal(err)
			}
			if !scoreboard.HasHazard(inst) {
				t.Fatal("CDNA3 accumulator should wait for v7")
			}
			scoreboard.Tick()
			if scoreboard.HasHazard(inst) {
				t.Fatal("CDNA3 accumulator still blocked after v7 became available")
			}
		})
	}
}

// The double-width list is keyed by decoded instruction names, so decode
// instructions taken from the MI300X fp32_throughput kernel to pin those names.
func TestFP32ThroughputKernelUsesDoubleWidthLatency(t *testing.T) {
	cases := []struct {
		name string
		code []byte
	}{
		{"v_pk_fma_f32", []byte{0x02, 0x00, 0xb0, 0xd3, 0x02, 0x0d, 0x18, 0x0c}},
		{"v_pk_add_f32", []byte{0x04, 0x40, 0xb2, 0xd3, 0x06, 0x09, 0x00, 0x10}},
		{"v_mov_b64_e32", []byte{0x04, 0x70, 0x0c, 0x7e}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := insts.NewDisassembler()
			d.IsCDNA3 = true
			inst, err := d.Decode(tc.code)
			if err != nil {
				t.Fatal(err)
			}
			if inst.InstName != tc.name {
				t.Fatalf("decoded %q, want %q", inst.InstName, tc.name)
			}
			if got := GetScoreboardLatency(inst); got != 8 {
				t.Fatalf("scoreboard latency = %d, want 8", got)
			}
		})
	}
}
