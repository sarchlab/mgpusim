package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALU(nil)
		alu.lds = make([]byte, 4096)

		state = newMockInstState()
	})

	It("should run DS_WRITE_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 13
		state.inst.Offset0 = 0
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Data = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0x01
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))
		state.WriteReg(insts.VReg(4), 1, 0, insts.Uint32ToBytes(1))

		alu.Run(state)

		lds := alu.LDS()
		Expect(insts.BytesToUint32(lds[100:])).To(Equal(uint32(1)))
	})

	It("should run DS_WRITE2_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 14
		state.inst.Offset0 = 0
		state.inst.Offset1 = 4
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Data = insts.NewVRegOperand(0, 4, 1)
		state.inst.Data1 = insts.NewVRegOperand(0, 8, 1)

		state.exec = 0x01
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))
		state.WriteReg(insts.VReg(4), 1, 0, insts.Uint32ToBytes(1))
		state.WriteReg(insts.VReg(8), 1, 0, insts.Uint32ToBytes(2))

		alu.Run(state)

		lds := alu.LDS()
		Expect(insts.BytesToUint32(lds[100:])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(lds[116:])).To(Equal(uint32(2)))
	})

	It("should run DS_WRITE_B8", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 30
		state.inst.Offset0 = 0
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Data = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0x01
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(201))
		state.WriteReg(insts.VReg(4), 1, 0, insts.Uint32ToBytes(0x12345678))

		alu.Run(state)

		lds := alu.LDS()
		Expect(lds[201]).To(Equal(uint8(0x78)))
	})

	It("should run DS_READ_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 54
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0x1
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))

		lds := alu.LDS()
		copy(lds[100:], insts.Uint32ToBytes(12))

		alu.Run(state)

		buf := state.ReadReg(insts.VReg(4), 1, 0)
		Expect(insts.BytesToUint32(buf)).To(Equal(uint32(12)))
	})

	It("should run DS_READ2_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 55
		state.inst.Offset0 = 0
		state.inst.Offset1 = 4
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)

		state.exec = 0x1
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))

		lds := alu.LDS()
		copy(lds[100:], insts.Uint32ToBytes(1))
		copy(lds[116:], insts.Uint32ToBytes(2))

		alu.Run(state)

		buf := state.ReadReg(insts.VReg(4), 2, 0)
		Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(2)))
	})

	It("should run DS_WRITE2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 78
		state.inst.Offset0 = 1
		state.inst.Offset1 = 3
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Data = insts.NewVRegOperand(0, 4, 2)
		state.inst.Data1 = insts.NewVRegOperand(0, 8, 2)

		state.exec = 0x1
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))
		// Data0: 8 bytes = dwords {1, 2}
		data0 := make([]byte, 8)
		copy(data0[0:4], insts.Uint32ToBytes(1))
		copy(data0[4:8], insts.Uint32ToBytes(2))
		state.WriteReg(insts.VReg(4), 2, 0, data0)
		// Data1: 8 bytes = dwords {3, 4}
		data1 := make([]byte, 8)
		copy(data1[0:4], insts.Uint32ToBytes(3))
		copy(data1[4:8], insts.Uint32ToBytes(4))
		state.WriteReg(insts.VReg(8), 2, 0, data1)

		alu.Run(state)

		lds := alu.LDS()
		// addr0 = 100 + 1*8 = 108
		Expect(insts.BytesToUint32(lds[108:])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(lds[112:])).To(Equal(uint32(2)))
		// addr1 = 100 + 3*8 = 124
		Expect(insts.BytesToUint32(lds[124:])).To(Equal(uint32(3)))
		Expect(insts.BytesToUint32(lds[128:])).To(Equal(uint32(4)))
	})

	It("should run DS_READ_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 118
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)

		state.exec = 0x1
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))

		lds := alu.LDS()
		copy(lds[100:], insts.Uint64ToBytes(12))

		alu.Run(state)

		buf := state.ReadReg(insts.VReg(4), 2, 0)
		Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(12)))
	})

	It("should run DS_READ2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 119
		state.inst.Offset0 = 1
		state.inst.Offset1 = 3
		state.inst.Addr = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 4)

		state.exec = 0x1
		state.WriteReg(insts.VReg(0), 1, 0, insts.Uint32ToBytes(100))

		lds := alu.LDS()
		// addr0 = 100 + 1*8 = 108
		copy(lds[108:], insts.Uint32ToBytes(12))
		// addr1 = 100 + 3*8 = 124
		copy(lds[124:], insts.Uint32ToBytes(156))

		alu.Run(state)

		buf := state.ReadReg(insts.VReg(4), 4, 0)
		// First 8 bytes from addr0
		Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(12)))
		// Second 8 bytes from addr1
		Expect(insts.BytesToUint32(buf[8:12])).To(Equal(uint32(156)))
	})

})
