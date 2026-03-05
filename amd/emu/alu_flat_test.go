package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"go.uber.org/mock/gomock"
)

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

	It("should run FLAT_LOAD_UBYTE", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().Find(vm.PID(1), uint64(i*4)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 16
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			// Write 64-bit address into v[0:1] for lane i
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*4)))
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf := state.ReadReg(insts.VReg(4), 1, i)
			val := insts.BytesToUint32(buf)
			Expect(val).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_SBYTE", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().Find(vm.PID(1), uint64(i*4)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 17
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*4)))
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(int8(i-128))))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			signedByte := int8(i - 128)
			extendedValue := int32(signedByte)
			buf := state.ReadReg(insts.VReg(4), 1, i)
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(extendedValue)))
		}
	})

	It("should run FLAT_LOAD_USHORT", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*4)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 18
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*4)))
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf := state.ReadReg(insts.VReg(4), 1, i)
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_DWORD", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*4)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 20
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*4)))
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf := state.ReadReg(insts.VReg(4), 1, i)
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_DWORDX2", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*8)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 21
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*8)))
			storage.Write(uint64(i*8), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*8+4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf := state.ReadReg(insts.VReg(4), 2, i)
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_DWORDX4", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*16)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 23
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 4)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*16)))
			storage.Write(uint64(i*16), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+4), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+8), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+12), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf := state.ReadReg(insts.VReg(4), 4, i)
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[8:12])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[12:16])).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORD", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*4)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 28
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Data = insts.NewVRegOperand(0, 4, 1)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*4)))
			state.WriteReg(insts.VReg(4), 1, i, insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*4), uint64(4))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORDX2", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*16)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 29
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Data = insts.NewVRegOperand(0, 4, 2)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*16)))
			buf := make([]byte, 8)
			copy(buf[0:4], insts.Uint32ToBytes(uint32(i)))
			copy(buf[4:8], insts.Uint32ToBytes(uint32(i)))
			state.WriteReg(insts.VReg(4), 2, i, buf)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*16), uint64(16))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORDX3", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*16)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 30
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Data = insts.NewVRegOperand(0, 4, 3)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*16)))
			buf := make([]byte, 12)
			copy(buf[0:4], insts.Uint32ToBytes(uint32(i)))
			copy(buf[4:8], insts.Uint32ToBytes(uint32(i)))
			copy(buf[8:12], insts.Uint32ToBytes(uint32(i)))
			state.WriteReg(insts.VReg(4), 3, i, buf)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*16), uint64(16))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[8:12])).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORDX4", func() {
		for i := 0; i < 64; i++ {
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(i*16)).
				Return(vm.Page{
					PAddr: uint64(0),
				}, true)
		}
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.FLAT
		state.inst.Opcode = 31
		state.inst.Addr = insts.NewVRegOperand(0, 0, 2)
		state.inst.Data = insts.NewVRegOperand(0, 4, 4)

		state.exec = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			state.WriteReg(insts.VReg(0), 2, i, insts.Uint64ToBytes(uint64(i*16)))
			buf := make([]byte, 16)
			copy(buf[0:4], insts.Uint32ToBytes(uint32(i)))
			copy(buf[4:8], insts.Uint32ToBytes(uint32(i)))
			copy(buf[8:12], insts.Uint32ToBytes(uint32(i)))
			copy(buf[12:16], insts.Uint32ToBytes(uint32(i)))
			state.WriteReg(insts.VReg(4), 4, i, buf)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*16), uint64(16))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf[0:4])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[4:8])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[8:12])).To(Equal(uint32(i)))
			Expect(insts.BytesToUint32(buf[12:16])).To(Equal(uint32(i)))
		}
	})
})
