package emu

import (
	"encoding/binary"
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/mem"
)

// A ComputeUnit is the unit that can execute workgroups.
//
// A ComputeUnit is a Yaotsu component
//   ToDispatcher <=> Receive the dispatch request and respond with the
//                    Completion signal
// 	 ToInstMem <=> Memory system for the instructions
//   ToDataMem <=> Memory system for the data in GPU memory
type ComputeUnit struct {
	*core.BasicComponent

	Freq             core.Freq
	Engine           core.Engine
	Disassembler     *disasm.Disassembler
	InstMem          core.Component
	DataMem          core.Component
	scalarInstWorker *ScalarInstWorker
	vectorInstWorker *VectorInstWorker

	WG     *WorkGroup
	co     *disasm.HsaCo
	packet *HsaKernelDispatchPacket
	grid   *Grid

	wiRegFile *mem.Storage
	wfRegFile *mem.Storage

	vgprPerWI     int
	sgprPerWf     int
	miscRegsBytes int
	wfRegByteSize int
	wiPerWf       int
	maxWI         int
	maxWf         int
}

// NewComputeUnit creates a ComputeUnit
func NewComputeUnit(name string,
	engine core.Engine,
	disassembler *disasm.Disassembler,
	scalarInstWorker *ScalarInstWorker,
	vectorInstWorker *VectorInstWorker,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.BasicComponent = core.NewBasicComponent(name)

	cu.Freq = 800 * core.MHz
	cu.Engine = engine
	cu.Disassembler = disassembler
	cu.scalarInstWorker = scalarInstWorker
	cu.vectorInstWorker = vectorInstWorker

	cu.vgprPerWI = 256
	cu.sgprPerWf = 102
	cu.miscRegsBytes = 114
	cu.wiPerWf = 64
	cu.maxWI = 1024
	cu.maxWf = cu.maxWI / cu.wiPerWf
	cu.wfRegByteSize = 4*(cu.sgprPerWf) + cu.miscRegsBytes

	cu.wiRegFile = mem.NewStorage(uint64(4 * cu.vgprPerWI * cu.maxWI))
	cu.wfRegFile = mem.NewStorage(uint64(cu.wfRegByteSize * cu.maxWf))

	cu.AddPort("ToDispatcher")
	cu.AddPort("ToInstMem")
	cu.AddPort("ToDataMem")

	return cu
}

// Receive processes the incomming requests
func (cu *ComputeUnit) Receive(req core.Request) *core.Error {
	switch req := req.(type) {
	case *MapWgReq:
		return cu.processMapWGReq(req)
	case *mem.AccessReq:
		return cu.processAccessReq(req)
	default:
		log.Panicf("ComputeUnit cannot process request of type %s", reflect.TypeOf(req))
		return core.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

func (cu *ComputeUnit) processMapWGReq(req *MapWgReq) *core.Error {
	// ComputeUnit is busy
	if cu.WG != nil {
		req.SwapSrcAndDst()
		req.IsReply = true
		req.Succeed = false
		cu.GetConnection("ToDispatcher").Send(req)
		return nil
	}

	// TODO: Change this part to a event
	cu.WG = req.WG
	cu.grid = cu.WG.Grid
	cu.co = cu.grid.CodeObject
	cu.packet = cu.grid.Packet

	cu.initRegs()
	cu.scheduleNextInst(cu.Freq.NextTick(req.RecvTime()))

	return nil
}

func (cu *ComputeUnit) processAccessReq(req *mem.AccessReq) *core.Error {
	info := req.Info.(*MemAccessInfo)
	if info.IsInstFetch {
		evt := NewEvalEvent()
		evt.SetHandler(cu)
		evt.SetTime(req.RecvTime())
		evt.Buf = req.Buf
		cu.Engine.Schedule(evt)
	}
	return nil
}

func (cu *ComputeUnit) initRegs() {
	cu.initSRegs()
	cu.initVRegs()
	cu.initMiscRegs()
}

func (cu *ComputeUnit) initSRegs() {
	numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
	for wiID := 0; wiID < numWi; wiID += cu.wiPerWf {
		cu.initSRegsForWf(wiID)
	}
}

func (cu *ComputeUnit) initSRegsForWf(wiID int) {
	count := 0
	if cu.co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Panic("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count += 4
	}

	if cu.co.EnableSgprDispatchPtr() {
		reg := disasm.SReg(count)
		bytes := make([]byte, 8)
		binary.PutUvarint(bytes, uint64(0))
		cu.WriteReg(reg, wiID, bytes)
		count += 2
	}

	if cu.co.EnableSgprQueuePtr() {
		log.Println("Initializing register QueuePtr is not supported")
		count += 2
	}

	if cu.co.EnableSgprKernelArgSegmentPtr() {
		reg := disasm.SReg(count)
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, uint64(cu.packet.KernargAddress))
		cu.WriteReg(reg, wiID, bytes)
		count += 2
	}

	if cu.co.EnableSgprDispatchId() {
		log.Println("Initializing register DispatchId is not supported")
		count += 2
	}

	if cu.co.EnableSgprFlatScratchInit() {
		log.Println("Initializing register FlatScratchInit is not supported")
		count += 2
	}

	if cu.co.EnableSgprPrivateSegementSize() {
		log.Println("Initializing register PrivateSegementSize is not supported")
		count++
	}

	if cu.co.EnableSgprGridWorkGroupCountX() {
		log.Println("Initializing register GridWorkGroupCountX is not supported")
		count++
	}

	if cu.co.EnableSgprGridWorkGroupCountY() {
		log.Println("Initializing register GridWorkGroupCountY is not supported")
		count++
	}

	if cu.co.EnableSgprGridWorkGroupCountZ() {
		log.Println("Initializing register GridWorkGroupCountZ is not supported")
		count++
	}

	if cu.co.EnableSgprWorkGroupIdX() {
		reg := disasm.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(cu.WG.IDX))
		cu.WriteReg(reg, wiID, bytes)
		count++
	}
	if cu.co.EnableSgprWorkGroupIdY() {
		reg := disasm.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(cu.WG.IDY))
		cu.WriteReg(reg, wiID, bytes)
		count++
	}
	if cu.co.EnableSgprWorkGroupIdZ() {
		reg := disasm.SReg(count)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(cu.WG.IDZ))
		cu.WriteReg(reg, wiID, bytes)
		count++
	}

	if cu.co.EnableSgprWorkGroupInfo() {
		log.Println("Initializing register GridWorkGroupInfo is not supported")
		count++
	}

	if cu.co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Println("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count++
	}

}

func (cu *ComputeUnit) initVRegs() {
	for x := 0; x < cu.WG.SizeX; x++ {
		for y := 0; y < cu.WG.SizeY; y++ {
			for z := 0; z < cu.WG.SizeZ; z++ {
				cu.initVRegsForWI(
					x, y, z, x+y*cu.WG.SizeX+z*cu.WG.SizeX*cu.WG.SizeY)
			}
		}
	}
}

func (cu *ComputeUnit) initVRegsForWI(
	wiIDX, wiIDY, wiIDZ, wiFlatID int) {
	reg := disasm.VReg(0)
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(wiIDX))
	cu.WriteReg(reg, wiFlatID, bytes)

	if cu.co.EnableVgprWorkItemId() > 0 {
		reg = disasm.VReg(1)
		bytes = make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(wiIDY))
		cu.WriteReg(reg, wiFlatID, bytes)
	}
	if cu.co.EnableVgprWorkItemId() > 1 {
		reg = disasm.VReg(2)
		bytes = make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(wiIDZ))
		cu.WriteReg(reg, wiFlatID, bytes)
	}
}

func (cu *ComputeUnit) initMiscRegs() {
	numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
	for wiID := 0; wiID < numWi; wiID += cu.wiPerWf {
		reg := disasm.Regs[disasm.Pc]
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, uint64(
			cu.packet.KernelObject+cu.co.KernelCodeEntryByteOffset))
		cu.WriteReg(reg, wiID, bytes)

		reg = disasm.Regs[disasm.Exec]
		bytes = make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, uint64(0xffffffff))
		cu.WriteReg(reg, wiID, bytes)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (cu *ComputeUnit) Handle(evt core.Event) error {
	cu.InvokeHook(evt, core.BeforeEvent)
	defer cu.InvokeHook(evt, core.AfterEvent)

	switch evt := evt.(type) {
	case *CUExeEvent:
		return cu.handleCUExeEvent(evt)
	case *EvalEvent:
		return cu.handleEvalEvent(evt)
	default:
		log.Panicf("event %s is not supported by component %s",
			reflect.TypeOf(evt), cu.Name())
	}
	return nil
}

func (cu *ComputeUnit) handleCUExeEvent(evt *CUExeEvent) error {
	fetchReq := mem.NewAccessReq()
	fetchReq.Address = cu.pc(0)
	fetchReq.ByteSize = 8
	fetchReq.SetSource(cu)
	fetchReq.SetDestination(cu.InstMem)
	fetchReq.Info = &MemAccessInfo{true}
	fetchReq.SetSendTime(evt.Time())
	err := cu.GetConnection("ToInstMem").Send(fetchReq)
	if err != nil {
		log.Panic(err)
	}

	evt.FinishChan() <- true
	return nil
}

// pc returns the program counter of a certain wavefront
func (cu *ComputeUnit) pc(wfID int) uint64 {
	data := cu.ReadReg(disasm.Regs[disasm.Pc], wfID*cu.wiPerWf, 8)
	return binary.LittleEndian.Uint64(data)
}

func (cu *ComputeUnit) handleEvalEvent(evt *EvalEvent) error {
	inst, err := cu.Disassembler.Decode(evt.Buf)
	if err != nil {
		log.Panic(err)
		return err
	}

	switch inst.FormatType {
	case disasm.Sop2, disasm.Sopk, disasm.Sop1, disasm.Sopc, disasm.Sopp:
		numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
		for wiFlatID := 0; wiFlatID < numWi; wiFlatID += cu.wiPerWf {
			cu.scalarInstWorker.Run(inst, wiFlatID)
		}
	case disasm.Vop1, disasm.Vop2:
		numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
		for wiFlatID := 0; wiFlatID < numWi; wiFlatID += cu.wiPerWf {
			cu.vectorInstWorker.Run(evt, wiFlatID)
		}
	case disasm.Flat:
		numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
		for wiFlatID := 0; wiFlatID < numWi; wiFlatID += cu.wiPerWf {
			cu.vectorInstWorker.Run(evt, wiFlatID)
		}

	default:
		log.Panicf("Instruction format %s is not supported.", inst.FormatName)

	}

	cu.scheduleNextInst(cu.Freq.NextTick(evt.Time()))

	evt.FinishChan() <- true
	return nil
}

func (cu *ComputeUnit) scheduleNextInst(time core.VTimeInSec) {
	evt := NewCUExeEvent()
	evt.SetHandler(cu)
	evt.SetTime(cu.Freq.NextTick(time))
	cu.Engine.Schedule(evt)
}

func (cu *ComputeUnit) dumpSRegs(wiFlatID int) {
	fmt.Printf("***** SRegs for wavefront %d *****\n", wiFlatID/cu.wiPerWf)
	for i := 0; i < cu.sgprPerWf; i++ {
		value := disasm.BytesToUint32(cu.ReadReg(disasm.SReg(i), wiFlatID, 4))
		if value != 0 {
			fmt.Printf("\ts%d 0x%08x\n", i, value)
		}
	}
	fmt.Printf("***** *****\n")
}

// WriteReg updates the value in the register file
func (cu *ComputeUnit) WriteReg(reg *disasm.Reg,
	wiFlatID int, data []byte) {
	if reg.IsVReg() {
		addr := cu.vgprAddr(reg, wiFlatID)
		err := cu.wiRegFile.Write(uint64(addr), data)
		if err != nil {
			log.Panic(err)
		}
	} else if reg.IsSReg() {
		addr := cu.sgprAddr(reg, wiFlatID)
		err := cu.wfRegFile.Write(uint64(addr), data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		addr := cu.miscRegAddr(reg, wiFlatID)
		err := cu.wfRegFile.Write(uint64(addr), data)
		if err != nil {
			log.Panic(err)
		}
	}
}

// ReadReg returns the register value in the register file
func (cu *ComputeUnit) ReadReg(reg *disasm.Reg,
	wiFlatID int, byteSize int) []byte {
	if reg.IsVReg() {
		addr := cu.vgprAddr(reg, wiFlatID)
		data, err := cu.wiRegFile.Read(uint64(addr), uint64(byteSize))
		if err != nil {
			log.Panic(err)
		}
		return data
	}

	if reg.IsSReg() {
		addr := cu.sgprAddr(reg, wiFlatID)
		data, err := cu.wfRegFile.Read(uint64(addr), uint64(byteSize))
		if err != nil {
			log.Panic(err)
		}
		return data
	}

	addr := cu.miscRegAddr(reg, wiFlatID)
	data, err := cu.wfRegFile.Read(uint64(addr), uint64(byteSize))
	if err != nil {
		log.Panic(err)
	}

	return data
}

// WriteMem provides convenient method to write into the GPU memory
func (cu *ComputeUnit) WriteMem(address uint64, data []byte) *core.Error {
	return nil
}

// ReadMem provides convenient method to read from the GPU memory
func (cu *ComputeUnit) ReadMem(address uint64, byteSize int) *core.Error {
	return nil
}

// vgprAddr converts a VGPR to the address in the vector register file
func (cu *ComputeUnit) vgprAddr(reg *disasm.Reg, wiFlatID int) int {
	return (wiFlatID*cu.vgprPerWI + reg.RegIndex()) * 4
}

// sgprAddr converts a SGPR to the address in the scalar register file
func (cu *ComputeUnit) sgprAddr(reg *disasm.Reg, wiFlatID int) int {
	wfID := wiFlatID / cu.wiPerWf
	return (wfID*cu.wfRegByteSize + reg.RegIndex()) * 4
}

// miscRegAddr returns the register's physical address in the scalar
// register file
func (cu *ComputeUnit) miscRegAddr(reg *disasm.Reg, wiFlatID int) int {
	wfID := wiFlatID / cu.wiPerWf
	offset := cu.wfRegByteSize * wfID
	switch reg {
	case disasm.Regs[disasm.Pc]:
		offset += 408 // 102 * 4
	case disasm.Regs[disasm.Exec]:
		offset += 416
	case disasm.Regs[disasm.Execz]:
		offset += 424
	case disasm.Regs[disasm.Vcc]:
		offset += 425
	case disasm.Regs[disasm.Vccz]:
		offset += 433
	case disasm.Regs[disasm.Scc]:
		offset += 434
	case disasm.Regs[disasm.FlatSratch]:
		offset += 435
	case disasm.Regs[disasm.XnackMask]:
		offset += 443
	case disasm.Regs[disasm.Status]:
		offset += 451
	case disasm.Regs[disasm.M0]:
		offset += 455
	case disasm.Regs[disasm.Trapsts]:
		offset += 459
	case disasm.Regs[disasm.Tma]:
		offset += 463
	case disasm.Regs[disasm.Timp0]:
		offset += 471
	case disasm.Regs[disasm.Timp1]:
		offset += 475
	case disasm.Regs[disasm.Timp2]:
		offset += 479
	case disasm.Regs[disasm.Timp3]:
		offset += 483
	case disasm.Regs[disasm.Timp4]:
		offset += 487
	case disasm.Regs[disasm.Timp5]:
		offset += 491
	case disasm.Regs[disasm.Timp6]:
		offset += 495
	case disasm.Regs[disasm.Timp7]:
		offset += 499
	case disasm.Regs[disasm.Timp8]:
		offset += 503
	case disasm.Regs[disasm.Timp9]:
		offset += 507
	case disasm.Regs[disasm.Timp10]:
		offset += 511
	case disasm.Regs[disasm.Timp11]:
		offset += 515
	case disasm.Regs[disasm.Vmcnt]:
		offset += 519
	case disasm.Regs[disasm.Expcnt]:
		offset += 520
	case disasm.Regs[disasm.Lgkmcnt]:
		offset += 521
	default:
		log.Panicf("Cannot find register %s's physical address", reg.Name)
	}
	return offset
}
