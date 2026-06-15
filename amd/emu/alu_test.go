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
	inst     *insts.Inst
	sRegFile []byte // 102 scalar registers * 4 bytes each
	vRegFile []byte // 256 vector registers * 4 bytes * 64 lanes
	exec     uint64
	vcc      uint64
	scc      byte
	pc       uint64
}

func newMockInstState() *mockInstState {
	s := new(mockInstState)
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
		state.inst.Base = insts.NewSRegOperand(0, 0, 2)
		state.inst.Offset = insts.NewIntOperand(0, 16)
		state.inst.Data = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(1024))

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Data, 0)
		Expect(uint32(dst)).To(Equal(uint32(217)))
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
		state.inst.Base = insts.NewSRegOperand(0, 0, 2)
		state.inst.Offset = insts.NewIntOperand(0, 16)
		state.inst.Data = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(1024))

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))

		alu.Run(state)

		// Read back the 2 dwords through reg file
		buf := state.ReadReg(insts.SReg(4), 2, 0)
		Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(217)))
		Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(218)))
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
		state.inst.Base = insts.NewSRegOperand(0, 0, 2)
		state.inst.Offset = insts.NewIntOperand(0, 16)
		state.inst.Data = insts.NewSRegOperand(0, 4, 4)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(1024))

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))
		storage.Write(uint64(1048), insts.Uint32ToBytes(219))
		storage.Write(uint64(1052), insts.Uint32ToBytes(220))

		alu.Run(state)

		buf := state.ReadReg(insts.SReg(4), 4, 0)
		Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(217)))
		Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(218)))
		Expect(insts.BytesToUint32(buf[8:12])).To(Equal(uint32(219)))
		Expect(insts.BytesToUint32(buf[12:16])).To(Equal(uint32(220)))
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
		state.inst.Base = insts.NewSRegOperand(0, 0, 2)
		state.inst.Offset = insts.NewIntOperand(0, 16)
		state.inst.Data = insts.NewSRegOperand(0, 4, 8)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(1024))

		for i := 0; i < 8; i++ {
			storage.Write(uint64(1040+i*4), insts.Uint32ToBytes(uint32(217+i)))
		}

		alu.Run(state)

		buf := state.ReadReg(insts.SReg(4), 8, 0)
		for i := 0; i < 8; i++ {
			Expect(insts.BytesToUint32(buf[i*4 : i*4+4])).To(Equal(uint32(217 + i)))
		}
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
		state.inst.Base = insts.NewSRegOperand(0, 0, 2)
		state.inst.Offset = insts.NewIntOperand(0, 16)
		state.inst.Data = insts.NewSRegOperand(0, 4, 16)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(1024))

		for i := 0; i < 16; i++ {
			storage.Write(uint64(1040+i*4), insts.Uint32ToBytes(uint32(217+i)))
		}

		alu.Run(state)

		buf := state.ReadReg(insts.SReg(4), 16, 0)
		for i := 0; i < 16; i++ {
			Expect(insts.BytesToUint32(buf[i*4 : i*4+4])).To(Equal(uint32(217 + i)))
		}
	})

	It("should run S_CBRANCH", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 2
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 2
		state.inst.SImm16 = insts.NewIntOperand(0, -32)

		state.SetPC(1024)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(1024 - 32*4)))
	})

	It("should run S_CBRANCH_SCC0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 4
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetSCC(0)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_SCC0, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 4
		state.inst.SImm16 = insts.NewIntOperand(0, -32)

		state.SetPC(1024)
		state.SetSCC(0)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(1024 - 32*4)))
	})

	It("should skip S_CBRANCH_SCC0, if SCC is 1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 4
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetSCC(1)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160)))
	})

	It("should run S_CBRANCH_SCC1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 5
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetSCC(1)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_SCC1, when IMM is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 5
		state.inst.SImm16 = insts.NewIntOperand(0, -32)

		state.SetPC(1024)
		state.SetSCC(1)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(1024 - 32*4)))
	})

	It("should skip S_CBRANCH_SCC1, if SCC is 0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 5
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetSCC(0)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160)))
	})

	It("should run S_CBRANCH_VCCZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 6
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetVCC(0)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_VCCNZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 7
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetVCC(0xffffffffffffffff)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_EXECZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 8
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetEXEC(0)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

	It("should run S_CBRANCH_EXECNZ", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPP
		state.inst.Opcode = 9
		state.inst.SImm16 = insts.NewIntOperand(0, 16)

		state.SetPC(160)
		state.SetEXEC(1)

		alu.Run(state)

		Expect(state.PC()).To(Equal(uint64(160 + 16*4)))
	})

})
