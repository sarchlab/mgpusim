package emu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
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

	Freq core.Freq

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

	if co.EnableSgprPrivateSegmentBuffer() {
		log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
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
		wf.PC += uint64(inst.ByteSize)

		log.Printf("wg - (%d, %d, %d), wf - %d, %s",
			wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID, inst)
		cu.instCount++

		if inst.FormatType == insts.Sopp && inst.Opcode == 10 { // S_ENDPGM
			wf.AtBarrier = true
			break
		}

		if inst.FormatType == insts.Sopp && inst.Opcode == 1 { // S_BARRIER
			wf.Completed = true
			break
		}

		cu.executeInst(wf)
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
