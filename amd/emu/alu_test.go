package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"go.uber.org/mock/gomock"
)

type mockInstState struct {
	inst       *insts.Inst
	scratchpad Scratchpad
	sRegFile   []byte // 102 scalar registers * 4 bytes each
	vRegFile   []byte // 256 vector registers * 4 bytes * 64 lanes
	exec       uint64
	vcc        uint64
	scc        byte
	pc         uint64
}

func newMockInstState() *mockInstState {
	s := new(mockInstState)
	s.scratchpad = make([]byte, 4096)
	s.sRegFile = make([]byte, 102*4)
	s.vRegFile = make([]byte, 256*4*64)
	return s
}

func (s *mockInstState) PID() vm.PID {
	return 1
}

func (s *mockInstState) Inst() *insts.Inst {
	return s.inst
}

func (s *mockInstState) Scratchpad() Scratchpad {
	return s.scratchpad
}

func (s *mockInstState) ReadReg(reg *insts.Reg, regCount int, laneID int) []byte {
	numBytes := reg.ByteSize
	if regCount >= 2 {
		numBytes *= regCount
	}
	value := make([]byte, numBytes)
	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(value, s.sRegFile[offset:offset+numBytes])
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(value, s.vRegFile[offset:offset+numBytes])
	} else if reg.RegType == insts.SCC {
		value[0] = s.scc
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		copy(value, insts.Uint64ToBytes(s.vcc))
	} else if reg.RegType == insts.VCCLO && regCount <= 1 {
		copy(value, insts.Uint32ToBytes(uint32(s.vcc)))
	} else if reg.RegType == insts.VCCHI && regCount <= 1 {
		copy(value, insts.Uint32ToBytes(uint32(s.vcc>>32)))
	} else if reg.RegType == insts.VCC {
		copy(value, insts.Uint64ToBytes(s.vcc))
	} else if reg.RegType == insts.EXEC || reg.RegType == insts.EXECLO {
		if numBytes >= 8 {
			copy(value, insts.Uint64ToBytes(s.exec))
		} else {
			copy(value, insts.Uint32ToBytes(uint32(s.exec)))
		}
	} else if reg.RegType == insts.EXECHI {
		copy(value, insts.Uint32ToBytes(uint32(s.exec>>32)))
	}
	return value
}

func (s *mockInstState) WriteReg(reg *insts.Reg, regCount int, laneID int, data []byte) {
	if reg.IsSReg() {
		offset := reg.RegIndex() * 4
		copy(s.sRegFile[offset:], data)
	} else if reg.IsVReg() {
		offset := laneID*256*4 + reg.RegIndex()*4
		copy(s.vRegFile[offset:], data)
	} else if reg.RegType == insts.SCC {
		s.scc = data[0]
	} else if reg.RegType == insts.VCC || reg.RegType == insts.VCCLO {
		if len(data) >= 8 {
			s.vcc = insts.BytesToUint64(data)
		} else {
			s.vcc = (s.vcc & 0xFFFFFFFF00000000) | uint64(insts.BytesToUint32(data))
		}
	} else if reg.RegType == insts.VCCHI {
		s.vcc = (s.vcc & 0x00000000FFFFFFFF) | (uint64(insts.BytesToUint32(data)) << 32)
	} else if reg.RegType == insts.EXEC || reg.RegType == insts.EXECLO {
		if len(data) >= 8 {
			s.exec = insts.BytesToUint64(data)
		} else {
			s.exec = (s.exec & 0xFFFFFFFF00000000) | uint64(insts.BytesToUint32(data))
		}
	} else if reg.RegType == insts.EXECHI {
		s.exec = (s.exec & 0x00000000FFFFFFFF) | (uint64(insts.BytesToUint32(data)) << 32)
	}
}

func (s *mockInstState) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		buf := s.ReadReg(operand.Register, operand.RegCount, laneID)
		// Pad to 8 bytes for BytesToUint64
		padded := make([]byte, 8)
		copy(padded, buf)
		return insts.BytesToUint64(padded)
	case insts.IntOperand:
		return uint64(operand.IntValue)
	case insts.FloatOperand:
		return uint64(operand.FloatValue)
	case insts.LiteralConstant:
		return uint64(operand.LiteralConstant)
	default:
		panic("unsupported operand type in mock")
	}
}

func (s *mockInstState) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if operand.OperandType != insts.RegOperand {
		panic("cannot write to non-register operand")
	}
	numBytes := operand.Register.ByteSize
	if operand.RegCount >= 2 {
		numBytes *= operand.RegCount
	}
	data := insts.Uint64ToBytes(value)
	s.WriteReg(operand.Register, operand.RegCount, laneID, data[:numBytes])
}

func (s *mockInstState) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		buf := s.ReadReg(operand.Register, operand.RegCount, laneID)
		if len(buf) > byteCount {
			return buf[:byteCount]
		}
		return buf
	case insts.IntOperand:
		data := insts.Uint64ToBytes(uint64(operand.IntValue))
		return data[:byteCount]
	default:
		panic("unsupported operand type in mock ReadOperandBytes")
	}
}

func (s *mockInstState) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	if operand.OperandType != insts.RegOperand {
		panic("cannot write to non-register operand")
	}
	s.WriteReg(operand.Register, operand.RegCount, laneID, data)
}

func (s *mockInstState) EXEC() uint64    { return s.exec }
func (s *mockInstState) SetEXEC(v uint64) { s.exec = v }
func (s *mockInstState) VCC() uint64     { return s.vcc }
func (s *mockInstState) SetVCC(v uint64)  { s.vcc = v }
func (s *mockInstState) SCC() byte       { return s.scc }
func (s *mockInstState) SetSCC(v byte)    { s.scc = v }
func (s *mockInstState) PC() uint64      { return s.pc }
func (s *mockInstState) SetPC(v uint64)   { s.pc = v }

var _ = Describe("ALU", func() {

	var (
		mockCtrl  *gomock.Controller
		pageTable *MockPageTable

		alu           *ALUImpl
		state         *mockInstState
		storage       *mem.Storage
		addrConverter *mem.InterleavingConverter
		sAccessor     StorageAccessor
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		pageTable = NewMockPageTable(mockCtrl)

		storage = mem.NewStorage(1 * mem.GB)
		addrConverter = &mem.InterleavingConverter{
			InterleavingSize:    1 * mem.GB,
			TotalNumOfElements:  1,
			CurrentElementIndex: 0,
			Offset:              0,
		}
		sAccessor = NewStorageAccessor(storage, pageTable, 12, addrConverter)
		alu = NewALU(sAccessor)

		state = newMockInstState()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should run S_LOAD_DWORD", func() {
		pageTable.EXPECT().
			Find(vm.PID(1), uint64(1040)).
			Return(vm.Page{
				PAddr: uint64(0),
			}, true)
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SMEM
		state.inst.Opcode = 0

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
	})

	It("should run S_LOAD_DWORDX2", func() {
		pageTable.EXPECT().
			Find(vm.PID(1), uint64(1040)).
			Return(vm.Page{
				PAddr: uint64(0),
			}, true)
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SMEM
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
		pageTable.EXPECT().
			Find(vm.PID(1), uint64(1040)).
			Return(vm.Page{
				PAddr: uint64(0),
			}, true)
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SMEM
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

	It("should run S_LOAD_DWORDX8", func() {
		pageTable.EXPECT().
			Find(vm.PID(1), uint64(1040)).
			Return(vm.Page{
				PAddr: uint64(0),
			}, true)
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SMEM
		state.inst.Opcode = 3

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))
		storage.Write(uint64(1048), insts.Uint32ToBytes(219))
		storage.Write(uint64(1052), insts.Uint32ToBytes(220))
		storage.Write(uint64(1056), insts.Uint32ToBytes(221))
		storage.Write(uint64(1060), insts.Uint32ToBytes(222))
		storage.Write(uint64(1064), insts.Uint32ToBytes(223))
		storage.Write(uint64(1068), insts.Uint32ToBytes(224))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
		Expect(layout.DST[1]).To(Equal(uint32(218)))
		Expect(layout.DST[2]).To(Equal(uint32(219)))
		Expect(layout.DST[3]).To(Equal(uint32(220)))
		Expect(layout.DST[4]).To(Equal(uint32(221)))
		Expect(layout.DST[5]).To(Equal(uint32(222)))
		Expect(layout.DST[6]).To(Equal(uint32(223)))
		Expect(layout.DST[7]).To(Equal(uint32(224)))
	})

	It("should run S_LOAD_DWORDX16", func() {
		pageTable.EXPECT().
			Find(vm.PID(1), uint64(1040)).
			Return(vm.Page{
				PAddr: uint64(0),
			}, true)
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SMEM
		state.inst.Opcode = 4

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))
		storage.Write(uint64(1048), insts.Uint32ToBytes(219))
		storage.Write(uint64(1052), insts.Uint32ToBytes(220))
		storage.Write(uint64(1056), insts.Uint32ToBytes(221))
		storage.Write(uint64(1060), insts.Uint32ToBytes(222))
		storage.Write(uint64(1064), insts.Uint32ToBytes(223))
		storage.Write(uint64(1068), insts.Uint32ToBytes(224))
		storage.Write(uint64(1072), insts.Uint32ToBytes(225))
		storage.Write(uint64(1076), insts.Uint32ToBytes(226))
		storage.Write(uint64(1080), insts.Uint32ToBytes(227))
		storage.Write(uint64(1084), insts.Uint32ToBytes(228))
		storage.Write(uint64(1088), insts.Uint32ToBytes(229))
		storage.Write(uint64(1092), insts.Uint32ToBytes(230))
		storage.Write(uint64(1096), insts.Uint32ToBytes(231))
		storage.Write(uint64(1100), insts.Uint32ToBytes(232))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
		Expect(layout.DST[1]).To(Equal(uint32(218)))
		Expect(layout.DST[2]).To(Equal(uint32(219)))
		Expect(layout.DST[3]).To(Equal(uint32(220)))
		Expect(layout.DST[4]).To(Equal(uint32(221)))
		Expect(layout.DST[5]).To(Equal(uint32(222)))
		Expect(layout.DST[6]).To(Equal(uint32(223)))
		Expect(layout.DST[7]).To(Equal(uint32(224)))
		Expect(layout.DST[8]).To(Equal(uint32(225)))
		Expect(layout.DST[9]).To(Equal(uint32(226)))
		Expect(layout.DST[10]).To(Equal(uint32(227)))
		Expect(layout.DST[11]).To(Equal(uint32(228)))
		Expect(layout.DST[12]).To(Equal(uint32(229)))
		Expect(layout.DST[13]).To(Equal(uint32(230)))
		Expect(layout.DST[14]).To(Equal(uint32(231)))
		Expect(layout.DST[15]).To(Equal(uint32(232)))
	})

	It("should run S_CBRANCH", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 2

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 2

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 1024
		layout.IMM = int64ToBits(-32)

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(1024 - 32*4)))
	})

	It("should run S_CBRANCH_SCC0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
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
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 8

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.EXEC = 0

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_EXECNZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 9

		layout := state.Scratchpad().AsSOPP()
		layout.PC = 160
		layout.IMM = 16
		layout.EXEC = 1

		alu.Run(state)

		Expect(layout.PC).To(Equal(uint64(160 + 16*4)))
	})

})
