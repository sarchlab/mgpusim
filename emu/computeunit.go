package emu

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/mem"
)

// A Decoder is a unit that can decode gcn3 instructions
type Decoder interface {
	Decode(buf []byte) (*disasm.Instruction, error)
}

// A ComputeUnit is the unit that can execute workgroups.
//
// A ComputeUnit is a Yaotsu component
//   ToDispatcher <=> Receive the dispatch request and respond with the
//                    Completion signal
// 	 ToInstMem <=> Memory system for the instructions
//   ToDataMem <=> Memory system for the data in GPU memory
type ComputeUnit struct {
	*core.BasicComponent

	// Connected Components
	InstMem core.Component
	DataMem core.Component

	// Properties
	Freq core.Freq

	// Dependencies
	Engine       core.Engine
	Disassembler Decoder
	RegInitiator *RegInitiator
	Scheduler    *Scheduler
	InstWorker   InstWorker

	WG     *WorkGroup
	co     *disasm.HsaCo
	packet *HsaKernelDispatchPacket
	grid   *Grid

	wiRegFile *mem.Storage
	wfRegFile *mem.Storage

	VgprPerWI     int
	SgprPerWf     int
	MiscRegsBytes int
	WfRegByteSize int
	WiPerWf       int
	MaxWI         int
	MaxWf         int

	Scheduling bool
}

// NewComputeUnit creates a ComputeUnit
func NewComputeUnit(name string,
	engine core.Engine,
	regInitiator *RegInitiator,
	scheduler *Scheduler,
	disassembler *disasm.Disassembler,
	instWorker InstWorker,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.BasicComponent = core.NewBasicComponent(name)

	cu.Freq = 800 * core.MHz
	cu.Engine = engine
	cu.RegInitiator = regInitiator
	cu.Scheduler = scheduler
	cu.Disassembler = disassembler
	cu.InstWorker = instWorker

	cu.VgprPerWI = 256
	cu.SgprPerWf = 102
	cu.MiscRegsBytes = 114
	cu.WiPerWf = 64
	cu.MaxWI = 1024
	cu.MaxWf = cu.MaxWI / cu.WiPerWf
	cu.WfRegByteSize = 4*(cu.SgprPerWf) + cu.MiscRegsBytes

	cu.wiRegFile = mem.NewStorage(uint64(4 * cu.VgprPerWI * cu.MaxWI))
	cu.wfRegFile = mem.NewStorage(uint64(cu.WfRegByteSize * cu.MaxWf))

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
		log.Panicf("CU %s is busy", cu.Name())
	}

	// TODO: Change this part to a event
	cu.WG = req.WG
	cu.grid = cu.WG.Grid
	cu.co = cu.grid.CodeObject
	cu.packet = cu.grid.Packet

	cu.RegInitiator.CU = cu
	cu.RegInitiator.Packet = cu.packet
	cu.RegInitiator.CO = cu.co
	cu.RegInitiator.WG = cu.WG
	cu.RegInitiator.InitRegs()

	numWi := cu.WG.SizeX * cu.WG.SizeY * cu.WG.SizeZ
	for wiID := 0; wiID < numWi; wiID += cu.WiPerWf {
		wf := NewWavefront()
		wf.FirstWiFlatID = wiID
		cu.Scheduler.AddWf(wf)
	}

	if !cu.Scheduling {
		cu.scheduleNextCycle(req.RecvTime())
		cu.Scheduling = true
	}

	return nil
}

func (cu *ComputeUnit) scheduleNextCycle(now core.VTimeInSec) {
	evt := NewScheduleEvent()
	evt.SetHandler(cu)
	evt.SetTime(cu.Freq.NextTick(now))
	cu.Engine.Schedule(evt)
}

func (cu *ComputeUnit) processAccessReq(req *mem.AccessReq) *core.Error {
	info := req.Info.(*MemAccessInfo)
	info.Ready = true
	if info.IsInstFetch {
		cu.Scheduler.Fetched(info.WfScheduleInfo, req.Buf)
	}
	if info.RegToSet != nil {
		cu.WriteReg(info.RegToSet, info.wiFlatID, req.Buf)
	}
	return nil
}

// Handle processes the events that is scheduled for the CommandProcessor
func (cu *ComputeUnit) Handle(evt core.Event) error {
	cu.InvokeHook(evt, core.BeforeEvent)
	defer cu.InvokeHook(evt, core.AfterEvent)

	switch evt := evt.(type) {
	case *ScheduleEvent:
		cu.Scheduler.Schedule(evt.Time())
		if cu.Scheduling {
			cu.scheduleNextCycle(evt.Time())
		}
		evt.FinishChan() <- true
		return nil
	default:
		log.Panicf("event %s is not supported by component %s",
			reflect.TypeOf(evt), cu.Name())
	}
	return nil
}

func (cu *ComputeUnit) dumpSRegs(wiFlatID int) {
	fmt.Printf("***** SRegs for wavefront %d *****\n", wiFlatID/cu.WiPerWf)
	for i := 0; i < cu.SgprPerWf; i++ {
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
func (cu *ComputeUnit) WriteMem(
	addr uint64, data []byte,
	info interface{}, now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	return nil, nil
}

// ReadMem provides convenient method to read from the GPU memory
func (cu *ComputeUnit) ReadMem(
	addr uint64, byteSize int,
	info interface{}, now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	req := mem.NewAccessReq()
	req.Type = mem.Read
	req.Address = addr
	req.ByteSize = uint64(byteSize)
	req.SetSource(cu)
	req.SetDestination(cu.DataMem)
	req.Info = info
	req.SetSendTime(now)
	err := cu.GetConnection("ToDataMem").Send(req)
	if err != nil && err.Recoverable == false {
		log.Panic(err)
	}
	return req, err
}

// ReadInstMem generate an event to the instruction memory
func (cu *ComputeUnit) ReadInstMem(
	addr uint64, byteSize int,
	info interface{}, now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	req := mem.NewAccessReq()
	req.Type = mem.Read
	req.Address = addr
	req.ByteSize = uint64(byteSize)
	req.SetSource(cu)
	req.SetDestination(cu.InstMem)
	req.Info = info
	req.SetSendTime(now)
	err := cu.GetConnection("ToInstMem").Send(req)
	if err != nil && err.Recoverable == false {
		log.Panic(err)
	}
	return req, err
}

// vgprAddr converts a VGPR to the address in the vector register file
func (cu *ComputeUnit) vgprAddr(reg *disasm.Reg, wiFlatID int) int {
	return (wiFlatID*cu.VgprPerWI + reg.RegIndex()) * 4
}

// sgprAddr converts a SGPR to the address in the scalar register file
func (cu *ComputeUnit) sgprAddr(reg *disasm.Reg, wiFlatID int) int {
	wfID := wiFlatID / cu.WiPerWf
	return (wfID*cu.WfRegByteSize + reg.RegIndex()) * 4
}

// miscRegAddr returns the register's physical address in the scalar
// register file
func (cu *ComputeUnit) miscRegAddr(reg *disasm.Reg, wiFlatID int) int {
	wfID := wiFlatID / cu.WiPerWf
	offset := cu.WfRegByteSize * wfID
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
