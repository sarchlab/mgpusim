package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

type mockInstState struct {
	inst       *insts.Inst
	scratchpad Scratchpad
}

func (s *mockInstState) Inst() *insts.Inst {
	return s.inst
}

func (s *mockInstState) Scratchpad() Scratchpad {
	return s.scratchpad
}

var _ = Describe("ALU", func() {

	var (
		alu     *ALUImpl
		state   *mockInstState
		storage *mem.Storage
	)

	BeforeEach(func() {
		storage = mem.NewStorage(1 * mem.GB)
		alu = NewALUImpl(storage)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run FLAT_LOAD_UBYTE", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 16

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+1]).To(Equal(uint32(0)))
			Expect(layout.DST[i*4+2]).To(Equal(uint32(0)))
			Expect(layout.DST[i*4+3]).To(Equal(uint32(0)))
		}

	})
	It("should run FLAT_LOAD_USHORT", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 18

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))

		}
	})

	It("should run FLAT_LOAD_DWORD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 20

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_DWORDX4", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 23

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			storage.Write(uint64(i*16), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+4), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+8), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+12), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+1]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+2]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+3]).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 28

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			layout.DATA[i*4] = uint32(i)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*4), uint64(4))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORDX4", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 31

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			layout.DATA[i*4] = uint32(i)
			layout.DATA[(i*4)+1] = uint32(i)
			layout.DATA[(i*4)+2] = uint32(i)
			layout.DATA[(i*4)+3] = uint32(i)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*16), uint64(16))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
		}
	})

	It("should run S_LOAD_DWORD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Smem
		state.inst.Opcode = 0

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
	})

	It("should run S_LOAD_DWORDX2", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Smem
		state.inst.Opcode = 1

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
		Expect(layout.DST[1]).To(Equal(uint32(218)))
	})
	It("should run S_LOAD_DWORDX4", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Smem
		state.inst.Opcode = 2

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))
		storage.Write(uint64(1048), insts.Uint32ToBytes(219))
		storage.Write(uint64(1052), insts.Uint32ToBytes(220))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
		Expect(layout.DST[1]).To(Equal(uint32(218)))
		Expect(layout.DST[2]).To(Equal(uint32(219)))
		Expect(layout.DST[3]).To(Equal(uint32(220)))

	})

	It("should run S_CBRANCH", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 2

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 2

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 1024
		layout.IMM = int64ToBits(-32)

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(1024 - 32*4)))
	})

	It("should run S_CBRANCH_SCC0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 4

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.SCC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_SCC0, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 4

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 1024
		layout.IMM = int64ToBits(-32)
		layout.SCC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(1024 - 32*4)))
	})

	It("should skip S_CBRANCH_SCC0, if SCC is 1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 4

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.SCC = 1

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160)))
	})

	It("should run S_CBRANCH_SCC1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 5

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.SCC = 1

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_SCC1, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 5

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 1024
		layout.IMM = int64ToBits(-32)
		layout.SCC = 1

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(1024 - 32*4)))
	})

	It("should skip S_CBRANCH_SCC1, if SCC is 0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 5

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.SCC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160)))
	})

	It("should run S_CBRANCH_VCCZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 6

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.VCC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_VCCNZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 7

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.VCC = 0xffffffffffffffff

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_EXECZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopp
		state.inst.Opcode = 8

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.EXEC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

})
