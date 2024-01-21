package emu

import (
	"encoding/binary"
	"log"
	"math"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/insts"
	"github.com/sarchlab/mgpusim/v3/kernels"
	"github.com/sarchlab/mgpusim/v3/protocol"
)

type emulationEvent struct {
	*sim.EventBase
}

// A ComputeUnit in the emu package is a component that omit the pipeline design
// but can still run the GCN3 instructions.
//
//	ToDispatcher <=> The port that connect the CU with the dispatcher
type ComputeUnit struct {
	*sim.TickingComponent

	decoder            Decoder
	scratchpadPreparer ScratchpadPreparer
	alu                ALU
	storageAccessor    *storageAccessor

	nextTick    sim.VTimeInSec
	queueingWGs []*protocol.MapWGReq
	wfs         map[*kernels.WorkGroup][]*Wavefront
	LDSStorage  []byte

	GlobalMemStorage *mem.Storage

	ToDispatcher sim.Port

	numCompletedWGs int
}

// ControlPort returns the port that can receive controlling messages from the
// Command Processor.
func (cu *ComputeUnit) ControlPort() sim.Port {
	return cu.ToDispatcher
}

// DispatchingPort returns the port that the dispatcher can use to dispatch
// work-groups to the CU.
func (cu *ComputeUnit) DispatchingPort() sim.Port {
	return cu.ToDispatcher
}

// WfPoolSizes returns an array of the numbers of wavefronts that each SIMD unit
// can execute.
func (cu *ComputeUnit) WfPoolSizes() []int {
	return []int{math.MaxInt32}
}

// VRegCounts returns an array of the numbers of vector regsiters in each SIMD
// unit.
func (cu *ComputeUnit) VRegCounts() []int {
	return []int{-1}
}

// SRegCount returns the number of scalar register in the Compute Unit.
func (cu *ComputeUnit) SRegCount() int {
	return -1
}

// LDSBytes returns the number of bytes in the LDS of the CU.
func (cu *ComputeUnit) LDSBytes() int {
	return -1
}

// Handle defines the behavior on event scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt sim.Event) error {
	cu.Lock()

	switch evt := evt.(type) {
	case sim.TickEvent:
		cu.TickingComponent.Handle(evt)
	case *emulationEvent:
		cu.runEmulation(evt)
	case *WGCompleteEvent:
		cu.handleWGCompleteEvent(evt)
	default:
		log.Panicf("cannot handle event %s", reflect.TypeOf(evt))
	}

	cu.Unlock()

	return nil
}

// Tick ticks
func (cu *ComputeUnit) Tick(now sim.VTimeInSec) bool {
	cu.processMapWGReq(now)
	return false
}

func (cu *ComputeUnit) processMapWGReq(now sim.VTimeInSec) {
	msg := cu.ToDispatcher.Retrieve(now)
	if msg == nil {
		return
	}

	req := msg.(*protocol.MapWGReq)

	if cu.nextTick <= now {
		cu.nextTick = sim.VTimeInSec(math.Ceil(float64(now)))
		//cu.nextTick = cu.Freq.NextTick(req.RecvTime())
		evt := &emulationEvent{
			sim.NewEventBase(cu.nextTick, cu),
		}
		cu.Engine.Schedule(evt)
	}

	cu.queueingWGs = append(cu.queueingWGs, req)
	cu.wfs[req.WorkGroup] = make([]*Wavefront, 0, 64)
}

func (cu *ComputeUnit) runEmulation(evt *emulationEvent) error {
	for len(cu.queueingWGs) > 0 {
		wg := cu.queueingWGs[0]
		cu.queueingWGs = cu.queueingWGs[1:]
		cu.runWG(wg, evt.Time())
	}
	return nil
}

func (cu *ComputeUnit) runWG(
	req *protocol.MapWGReq,
	now sim.VTimeInSec,
) error {
	wg := req.WorkGroup
	cu.initWfs(wg, req)

	for !cu.isAllWfCompleted(wg) {
		for _, wf := range cu.wfs[wg] {
			cu.alu.SetLDS(wf.LDS)
			cu.runWfUntilBarrier(wf)
		}
		cu.resolveBarrier(wg)
	}

	cu.numCompletedWGs++
	evt := NewWGCompleteEvent(cu.Freq.NextTick(now), cu, req)
	cu.Engine.Schedule(evt)

	return nil
}

func (cu *ComputeUnit) initWfs(
	wg *kernels.WorkGroup,
	req *protocol.MapWGReq,
) error {
	lds := cu.initLDS(wg, req)

	for _, wf := range wg.Wavefronts {
		managedWf := NewWavefront(wf)
		managedWf.LDS = lds
		managedWf.pid = req.PID
		cu.wfs[wg] = append(cu.wfs[wg], managedWf)
	}

	for _, managedWf := range cu.wfs[wg] {
		cu.initWfRegs(managedWf)
	}

	return nil
}

func (cu *ComputeUnit) initLDS(wg *kernels.WorkGroup, req *protocol.MapWGReq) []byte {
	ldsSize := req.WorkGroup.Packet.GroupSegmentSize
	lds := make([]byte, ldsSize)
	return lds
}

//nolint:funlen,gocyclo
func (cu *ComputeUnit) initWfRegs(wf *Wavefront) {
	co := wf.CodeObject
	pkt := wf.Packet

	wf.PC = pkt.KernelObject + co.KernelCodeEntryByteOffset
	wf.Exec = wf.InitExecMask

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer() {
		// log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		//fmt.Printf("s%d SGPRPrivateSegmentBuffer\n", SGPRPtr/4)
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], wf.PacketAddress)
		//fmt.Printf("s%d SGPRDispatchPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr() {
		log.Printf("EnableSgprQueuePtr is not supported")
		//fmt.Printf("s%d SGPRQueuePtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], pkt.KernargAddress)
		//fmt.Printf("s%d SGPRKernelArgSegmentPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchID() {
		log.Printf("EnableSgprDispatchID is not supported")
		//fmt.Printf("s%d SGPRDispatchID\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit() {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		//fmt.Printf("s%d SGPRFlatScratchInit\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		//fmt.Printf("s%d SGPRPrivateSegmentSize\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeX+uint32(pkt.WorkgroupSizeX)-1)/uint32(pkt.WorkgroupSizeX))
		//fmt.Printf("s%d WorkGroupCountX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeY+uint32(pkt.WorkgroupSizeY)-1)/uint32(pkt.WorkgroupSizeY))
		//fmt.Printf("s%d WorkGroupCountY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeZ+uint32(pkt.WorkgroupSizeZ)-1)/uint32(pkt.WorkgroupSizeZ))
		//fmt.Printf("s%d WorkGroupCountZ\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDX))
		//fmt.Printf("s%d WorkGroupIdX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDY))
		//fmt.Printf("s%d WorkGroupIdY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIDZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDZ))
		//fmt.Printf("s%d WorkGroupIdZ\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		SGPRPtr += 4
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Printf("EnableSgprPrivateSegentWaveByteOffset is not supported")
		SGPRPtr += 4
	}

	var x, y, z int
	for i := wf.FirstWiFlatID; i < wf.FirstWiFlatID+64; i++ {
		z = i / (wf.WG.SizeX * wf.WG.SizeY)
		y = i % (wf.WG.SizeX * wf.WG.SizeY) / wf.WG.SizeX
		x = i % (wf.WG.SizeX * wf.WG.SizeY) % wf.WG.SizeX
		laneID := i - wf.FirstWiFlatID

		wf.WriteReg(insts.VReg(0), 1, laneID, insts.Uint32ToBytes(uint32(x)))

		if co.EnableVgprWorkItemID() > 0 {
			wf.WriteReg(insts.VReg(1), 1, laneID, insts.Uint32ToBytes(uint32(y)))
		}

		if co.EnableVgprWorkItemID() > 1 {
			wf.WriteReg(insts.VReg(2), 1, laneID, insts.Uint32ToBytes(uint32(z)))
		}
	}
}

func (cu *ComputeUnit) isAllWfCompleted(wg *kernels.WorkGroup) bool {
	for _, wf := range cu.wfs[wg] {
		if !wf.Completed {
			return false
		}
	}
	return true
}

func (cu *ComputeUnit) runWfUntilBarrier(wf *Wavefront) error {
	for {
		instBuf := cu.storageAccessor.Read(wf.pid, wf.PC, 8)

		inst, _ := cu.decoder.Decode(instBuf)
		wf.inst = inst

		wf.PC += uint64(inst.ByteSize)

		if inst.FormatType == insts.SOPP && inst.Opcode == 10 { // S_ENDPGM
			wf.AtBarrier = true
			cu.logInst(wf, inst)
			break
		}

		if inst.FormatType == insts.SOPP && inst.Opcode == 1 { // S_BARRIER
			wf.Completed = true
			cu.logInst(wf, inst)
			break
		}

		cu.executeInst(wf)
		cu.logInst(wf, inst)
	}

	return nil
}

func (cu *ComputeUnit) logInst(wf *Wavefront, inst *insts.Inst) {
	ctx := sim.HookCtx{
		Domain: cu,
		Item:   wf,
		Detail: inst,
	}
	cu.InvokeHook(ctx)
}

func (cu *ComputeUnit) executeInst(wf *Wavefront) {
	cu.scratchpadPreparer.Prepare(wf, wf)
	cu.alu.Run(wf)
	cu.scratchpadPreparer.Commit(wf, wf)
}

func (cu *ComputeUnit) resolveBarrier(wg *kernels.WorkGroup) {
	if cu.isAllWfCompleted(wg) {
		return
	}

	for _, wf := range cu.wfs[wg] {
		if !wf.AtBarrier {
			log.Panic("not all wavefronts at barrier")
		}
		wf.AtBarrier = false
	}
}

func (cu *ComputeUnit) handleWGCompleteEvent(evt *WGCompleteEvent) error {
	delete(cu.wfs, evt.Req.WorkGroup)

	if len(cu.wfs) != 0 {
		return nil
	}

	req := protocol.EmuAllWGCompletionMsgBuilder{}.
		WithSrc(cu.ToDispatcher).
		WithDst(evt.Req.Src).
		WithSendTime(evt.Time()).
		WithNumWGs(cu.numCompletedWGs).
		Build()

	err := cu.ToDispatcher.Send(req)
	if err != nil {
		newEvent := NewWGCompleteEvent(cu.Freq.NextTick(evt.Time()),
			cu, evt.Req)
		cu.Engine.Schedule(newEvent)
	}

	return nil
}

// NewComputeUnit creates a new ComputeUnit with the given name
func NewComputeUnit(
	name string,
	engine sim.Engine,
	decoder Decoder,
	scratchpadPreparer ScratchpadPreparer,
	alu ALU,
	sAccessor *storageAccessor,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.TickingComponent = sim.NewTickingComponent(name,
		engine, 1*sim.GHz, cu)

	cu.decoder = decoder
	cu.scratchpadPreparer = scratchpadPreparer
	cu.alu = alu
	cu.storageAccessor = sAccessor

	cu.queueingWGs = make([]*protocol.MapWGReq, 0)
	cu.wfs = make(map[*kernels.WorkGroup][]*Wavefront)

	cu.ToDispatcher = sim.NewLimitNumMsgPort(cu, 1, name+".ToDispatcher")

	return cu
}

// BuildComputeUnit build a compute unit
func BuildComputeUnit(
	name string,
	engine sim.Engine,
	decoder Decoder,
	pageTable vm.PageTable,
	log2PageSize uint64,
	storage *mem.Storage,
	addrConverter mem.AddressConverter,
) *ComputeUnit {
	scratchpadPreparer := NewScratchpadPreparerImpl()
	sAccessor := newStorageAccessor(
		storage, pageTable, log2PageSize, addrConverter)
	alu := NewALU(sAccessor)
	cu := NewComputeUnit(name, engine, decoder,
		scratchpadPreparer, alu, sAccessor)
	return cu
}
