package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALUImpl(nil)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run v_cmp_lt_f32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 65

		sp := state.Scratchpad().AsVOP3A()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_gt_i32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 196

		sp := state.Scratchpad().AsVOP3A()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(int32ToBits(-1))
		sp.SRC1[0] = uint64(int32ToBits(1))
		sp.SRC0[1] = uint64(int32ToBits(2))
		sp.SRC1[1] = uint64(int32ToBits(1))
		sp.SRC0[2] = uint64(int32ToBits(0))
		sp.SRC1[2] = uint64(int32ToBits(-1))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x6)))
	})

	It("should run V_CMP_LT_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 201

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(2)))
	})

	It("should run V_CMP_EQ_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 202

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.EXEC = 3

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(1)))
	})

	It("should run V_CMP_LE_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 203

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(3)))
	})

	It("should run V_CMP_GT_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 204

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(4)))
	})

	It("should run V_CMP_LG_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 205

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(6)))
	})

	It("should run V_CMP_GE_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 206

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(5)))
	})

	It("should run V_CMP_LT_U64 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 233

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.EXEC = 0x7

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(2)))
	})

	It("should run V_CNDMASK_B32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 256

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC2[0] = 1
		sp.EXEC = 3

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(2)))
		Expect(sp.DST[1]).To(Equal(uint64(1)))
	})

	It("should run V_MAD_U32_U24", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 451

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 10
		sp.SRC1[0] = 20
		sp.SRC2[0] = 50
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(250)))

	})

	It("should run V_MAD_U64_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 488

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = 10
		sp.SRC1[0] = 20
		sp.SRC2[0] = 50
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(250)))
	})

	It("should run V_MUL_LO_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 645

		sp := state.Scratchpad().AsVOP3A()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(i)
			sp.SRC1[i] = uint64(2)
		}
		sp.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(sp.DST[i]).To(Equal(uint64(i * 2)))
		}
	})

	It("should run V_MUL_HI_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 646

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = uint64(0x80000000)
		sp.SRC1[0] = uint64(2)
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(1)))

	})

	It("should run V_LSHLREV_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 655

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC1[0] = uint64(0x0000000000010000)
		sp.SRC0[0] = uint64(3)
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x0000000000080000)))
	})

	It("should run V_ASHRREV_I64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 657

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC1[0] = uint64(0x0000000000010000)
		sp.SRC1[1] = uint64(0xffffffff00010000)
		sp.SRC0[0] = 4
		sp.SRC0[1] = 4
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x0000000000001000)))
		Expect(sp.DST[1]).To(Equal(uint64(0xfffffffff0001000)))
	})
})
