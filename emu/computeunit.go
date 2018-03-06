package emu

import (
	"log"
	"math"
	"reflect"

	"encoding/binary"

	"sync"

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
	sync.Mutex

	engine             core.Engine
	decoder            Decoder
	scratchpadPreparer ScratchpadPreparer
	alu                ALU

	Freq util.Freq

	nextTick    core.VTimeInSec
	queueingWGs []*gcn3.MapWGReq
	wfs         map[*kernels.WorkGroup][]*Wavefront
	LDSStorage  []byte

	GlobalMemStorage *mem.Storage
}

// NewComputeUnit creates a new ComputeUnit with the given name
func NewComputeUnit(
	name string,
	engine core.Engine,
	decoder Decoder,
	scratchpadPreparer ScratchpadPreparer,
	alu ALU,
) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.ComponentBase = core.NewComponentBase(name)

	cu.engine = engine
	cu.decoder = decoder
	cu.scratchpadPreparer = scratchpadPreparer
	cu.alu = alu

	cu.queueingWGs = make([]*gcn3.MapWGReq, 0)
	cu.wfs = make(map[*kernels.WorkGroup][]*Wavefront)

	cu.AddPort("ToDispatcher")

	return cu
}

// Recv accepts requests from other components
func (cu *ComputeUnit) Recv(req core.Req) *core.Error {
	util.ProcessReqAsEvent(req, cu.engine, cu.Freq)
	return nil
}

// Handle defines the behavior on event scheduled on the ComputeUnit
func (cu *ComputeUnit) Handle(evt core.Event) error {
	cu.Lock()
	defer cu.Unlock()

	switch evt := evt.(type) {
	case *gcn3.MapWGReq:
		return cu.handleMapWGReq(evt)
	case *gcn3.DispatchWfReq:
		return cu.handleDispatchWfReq(evt)
	case *core.TickEvent:
		return cu.handleTickEvent(evt)
	case *WGCompleteEvent:
		return cu.handleWGCompleteEvent(evt)
	default:
		log.Panicf("cannot handle event %s", reflect.TypeOf(evt))
	}
	return nil
}

func (cu *ComputeUnit) handleMapWGReq(req *gcn3.MapWGReq) error {
	if cu.nextTick <= req.Time() {
		cu.nextTick = core.VTimeInSec(math.Ceil(float64(req.RecvTime())))
		evt := core.NewTickEvent(
			cu.nextTick,
			cu,
		)
		cu.engine.Schedule(evt)
	}

	cu.queueingWGs = append(cu.queueingWGs, req)
	cu.wfs[req.WG] = make([]*Wavefront, 0, 64)

	req.Ok = true
	req.SwapSrcAndDst()
	req.SetSendTime(req.Time())
	cu.GetConnection("ToDispatcher").Send(req)

	return nil
}

func (cu *ComputeUnit) handleDispatchWfReq(req *gcn3.DispatchWfReq) error {
	req.SwapSrcAndDst()
	req.SetSendTime(req.Time())
	cu.GetConnection("ToDispatcher").Send(req)
	return nil
}

func (cu *ComputeUnit) handleTickEvent(evt *core.TickEvent) error {

	for len(cu.queueingWGs) > 0 {
		wg := cu.queueingWGs[0]
		cu.queueingWGs = cu.queueingWGs[1:]
		cu.runWG(wg, evt.Time())
	}
	return nil
	//wg := cu.running.WG
	//
	//if cu.running != nil {
	//	return cu.runWG(wg, evt.Time())
	//}
	//
	//return nil
}

func (cu *ComputeUnit) runWG(req *gcn3.MapWGReq, now core.VTimeInSec) error {
	wg := req.WG
	cu.initWfs(wg)

	for !cu.isAllWfCompleted(wg) {
		for _, wf := range cu.wfs[wg] {
			cu.runWfUntilBarrier(wf)
		}
		cu.resolveBarrier(wg)
	}

	evt := NewWGCompleteEvent(cu.Freq.NextTick(now), cu, req)
	cu.engine.Schedule(evt)

	return nil
}

func (cu *ComputeUnit) initWfs(wg *kernels.WorkGroup) error {
	for _, wf := range wg.Wavefronts {
		managedWf := NewWavefront(wf)
		cu.wfs[wg] = append(cu.wfs[wg], managedWf)
	}

	for _, managedWf := range cu.wfs[wg] {
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
		// log.Printf("EnableSgprPrivateSegmentBuffer is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentBuffer\n", SGPRPtr/4)
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], wf.PacketAddress)
		// fmt.Printf("s%d SGPRDispatchPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr() {
		log.Printf("EnableSgprQueuePtr is not supported")
		// fmt.Printf("s%d SGPRQueuePtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], pkt.KernargAddress)
		// fmt.Printf("s%d SGPRKernelArgSegmentPtr\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchId() {
		log.Printf("EnableSgprDispatchID is not supported")
		// fmt.Printf("s%d SGPRDispatchID\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit() {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		// fmt.Printf("s%d SGPRFlatScratchInit\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		// fmt.Printf("s%d SGPRPrivateSegmentSize\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeX+uint32(pkt.WorkgroupSizeX)-1)/uint32(pkt.WorkgroupSizeX))
		// fmt.Printf("s%d WorkGroupCountX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeY+uint32(pkt.WorkgroupSizeY)-1)/uint32(pkt.WorkgroupSizeY))
		// fmt.Printf("s%d WorkGroupCountY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeZ+uint32(pkt.WorkgroupSizeZ)-1)/uint32(pkt.WorkgroupSizeZ))
		// fmt.Printf("s%d WorkGroupCountZ\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdX() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDX))
		// fmt.Printf("s%d WorkGroupIdX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdY() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDY))
		// fmt.Printf("s%d WorkGroupIdY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupIdZ() {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			uint32(wf.WG.IDZ))
		// fmt.Printf("s%d WorkGroupIdZ\n", SGPRPtr/4)
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
		instBuf, err := cu.GlobalMemStorage.Read(wf.PC, 8)
		if err != nil {
			log.Fatal(err)
		}

		inst, err := cu.decoder.Decode(instBuf)
		wf.inst = inst

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
	delete(cu.wfs, evt.Req.WG)
	req := gcn3.NewWGFinishMesg(cu, evt.Req.Dst(), evt.Time(), evt.Req.WG)
	cu.GetConnection("ToDispatcher").Send(req)
	return nil
}
