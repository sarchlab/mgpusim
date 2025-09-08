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
		sAccessor     *StorageAccessor
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

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}
		layout.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+1]).To(Equal(uint32(0)))
			Expect(layout.DST[i*4+2]).To(Equal(uint32(0)))
			Expect(layout.DST[i*4+3]).To(Equal(uint32(0)))
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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}
		layout.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))

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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}
		layout.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 8)
			storage.Write(uint64(i*8), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*8+4), insts.Uint32ToBytes(uint32(i)))
		}
		layout.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+1]).To(Equal(uint32(i)))
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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			storage.Write(uint64(i*16), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+4), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+8), insts.Uint32ToBytes(uint32(i)))
			storage.Write(uint64(i*16+12), insts.Uint32ToBytes(uint32(i)))
		}
		layout.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+1]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+2]).To(Equal(uint32(i)))
			Expect(layout.DST[i*4+3]).To(Equal(uint32(i)))
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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			layout.DATA[i*4] = uint32(i)
		}
		layout.EXEC = 0xffffffffffffffff

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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			layout.DATA[i*4] = uint32(i)
			layout.DATA[(i*4)+1] = uint32(i)
		}
		layout.EXEC = 0xffffffffffffffff

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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			layout.DATA[i*4] = uint32(i)
			layout.DATA[(i*4)+1] = uint32(i)
			layout.DATA[(i*4)+2] = uint32(i)
		}
		layout.EXEC = 0xffffffffffffffff

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

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 16)
			layout.DATA[i*4] = uint32(i)
			layout.DATA[(i*4)+1] = uint32(i)
			layout.DATA[(i*4)+2] = uint32(i)
			layout.DATA[(i*4)+3] = uint32(i)
		}
		layout.EXEC = 0xffffffffffffffff

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
