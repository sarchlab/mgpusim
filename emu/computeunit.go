package emu

import (
	"log"
	"reflect"

	"encoding/binary"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"
)

// A ComputeUnit in the emu package is a component that omit the pipeline design
// but can still run the GCN3 instructions.
//
//     ToDispatcher <=> The port that connect the CU with the dispatcher
//
type ComputeUnit struct {
	*core.ComponentBase

	engine             core.Engine
	decoder            Decoder
	scratchpadPreparer ScratchpadPreparer
	alu                *ALU

	Freq util.Freq

	running    *gcn3.MapWGReq
	wfs        []*Wavefront
	instCount  int
	LDSStorage []byte

	GlobalMemStorage *mem.Storage
}

// NewComputeUnit creates a new ComputeUnit with the given name
func NewComputeUnit(
	name string,
	engine core.Engine,
	decoder Decoder,
	scratchpadPreparer ScratchpadPreparer,
	alu *ALU,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine
	cu.decoder = decoder
	cu.scratchpadPreparer = scratchpadPreparer
	cu.alu = alu

	cu.wfs = make([]*Wavefront, 0)

	cu.AddPort("ToDispatcher")

	return cu
}

// Recv accepts requests from other components
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *gcn3.MapWGReq:
		return cu.processMapWGReq(req)
	default:
		log.Panicf("cannot process req %s", reflect.TypeOf(req))
	}
	return nil
}

func (cu *ComputeUnit) processMapWGReq(req *gcn3.MapWGReq) *core.Error {
	if cu.running != nil {
		req.Ok = false
	} else {
		req.Ok = true
		cu.running = req
		cu.instCount = 0

		evt := core.NewTickEvent(req.RecvTime(), cu)
		cu.engine.Schedule(evt)
	}

	req.SwapSrcAndDst()
	req.SetSendTime(cu.Freq.HalfTick(req.RecvTime()))
	deferredSend := core.NewDeferredSend(req)
	cu.engine.Schedule(deferredSend)

	return nil
}

// Handle defines the behavior on event scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *core.TickEvent:
		return cu.handleTickEvent(evt)
	case *WGCompleteEvent:
		return cu.handleWGCompleteEvent(evt)
	case *core.DeferredSend:
		return cu.handleDeferredSend(evt)
	default:
		log.Panicf("cannot handle event %s", reflect.TypeOf(evt))
	}
	return nil
}

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {
	wg := cu.running.WG

	if cu.running != nil {
		return cu.runWG(wg, evt.Time())
	}

	return nil
}

func (cu *ComputeUnit) runWG(wg *kernels.WorkGroup, now core.VTimeInSec) error {
	cu.wfs = nil
	cu.initWfs(wg)

	for !cu.isAllWfCompleted() {
		for _, wf := range cu.wfs {
			cu.runWfUntilBarrier(wf)
		}
		cu.resolveBarrier()
	}

	evt := NewWGCompleteEvent(cu.Freq.NCyclesLater(cu.instCount, now), cu, wg)
	cu.engine.Schedule(evt)

	return nil
}

func (cu *ComputeUnit) initWfs(wg *kernels.WorkGroup) error {
	for _, wf := range wg.Wavefronts {
		managedWf := NewWavefront(wf)
		cu.wfs = append(cu.wfs, managedWf)
	}

	for _, managedWf := range cu.wfs {
		cu.initWfRegs(managedWf)
	}

	return nil
}

func (cu *ComputeUnit) initWfRegs(wf *Wavefront) {
	co := wf.CodeObject
	pkt := wf.Packet

	wf.PC = pkt.KernelObject + co.KernelCodeEntryByteOffset
	wf.Exec = 0xffffffffffffffff

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer() {
		log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], wf.PacketAddress)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr() {
		log.Printf("EnableSgprQueuePtr is not supported")
		SGPRPtr += 8
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], pkt.KernargAddress)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchId() {
		log.Printf("EnableSgprDispatchID is not supported")
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit() {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeX+uint32(pkt.WorkgroupSizeX)-1)/uint32(pkt.WorkgroupSizeX))
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeY+uint32(pkt.WorkgroupSizeY)-1)/uint32(pkt.WorkgroupSizeY))
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeZ+uint32(pkt.WorkgroupSizeZ)-1)/uint32(pkt.WorkgroupSizeZ))
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDX))
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDY))
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDZ))
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

		if co.EnableVgprWorkItemId() > 0 {
			wf.WriteReg(insts.VReg(1), 1, laneID, insts.Uint32ToBytes(uint32(y)))
		}

		if co.EnableVgprWorkItemId() > 1 {
			wf.WriteReg(insts.VReg(2), 1, laneID, insts.Uint32ToBytes(uint32(z)))
		}
	}
}

func (cu *ComputeUnit) isAllWfCompleted() bool {
	for _, wf := range cu.wfs {
		if !wf.Completed {
			return false
		}
	}
	return true
}

func (cu *ComputeUnit) runWfUntilBarrier(wf *Wavefront) error {
	for {
		instBuf, err := cu.GlobalMemStorage.Read(wf.PC, 8)
		if err != nil {
			log.Fatal(err)
		}

		inst, err := cu.decoder.Decode(instBuf)
		wf.inst = inst

		cu.instCount++

		wf.PC += uint64(inst.ByteSize)

		if inst.FormatType == insts.Sopp && inst.Opcode == 10 { // S_ENDPGM
			wf.AtBarrier = true
			break
		}

		if inst.FormatType == insts.Sopp && inst.Opcode == 1 { // S_BARRIER
			wf.Completed = true
			break
		}

		cu.executeInst(wf)
		cu.InvokeHook(wf, cu, core.Any, inst)
	}

	return nil
}

func (cu *ComputeUnit) executeInst(wf *Wavefront) {
	cu.scratchpadPreparer.Prepare(wf, wf)
	cu.alu.Run(wf)
	cu.scratchpadPreparer.Commit(wf, wf)
}

func (cu *ComputeUnit) resolveBarrier() {
	if cu.isAllWfCompleted() {
		return
	}

	for _, wf := range cu.wfs {
		if !wf.AtBarrier {
			log.Panic("not all wavefronts at barrier")
		}
		wf.AtBarrier = false
	}
}

func (cu *ComputeUnit) handleWGCompleteEvent(evt *WGCompleteEvent) error {
	req := gcn3.NewWGFinishMesg(cu, cu.running.Dst(), evt.Time(), cu.running.WG)
	cu.GetConnection("ToDispatcher").Send(req)
	cu.running = nil
	return nil
}

func (cu *ComputeUnit) handleDeferredSend(evt *core.DeferredSend) error {
	req := evt.Req
	cu.GetConnection("ToDispatcher").Send(req)
	return nil
}
