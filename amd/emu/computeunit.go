package emu

import (
	"encoding/binary"
	"log"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

const psPerSecond = timing.VTimeInPicoSec(1_000_000_000_000)

// ceilToSecond rounds the time up to the next whole virtual second. The
// emulation ComputeUnit batches all the work-groups that arrive before a
// whole-second boundary into a single emulation pass (matching the v4
// behavior of scheduling the emulation event at math.Ceil(now)).
func ceilToSecond(t timing.VTimeInPicoSec) timing.VTimeInPicoSec {
	return (t + psPerSecond - 1) / psPerSecond * psPerSecond
}

// pendingWGCompletion is a work-group that finished emulation and whose
// completion is scheduled at a future time.
type pendingWGCompletion struct {
	completeAt timing.VTimeInPicoSec
	reqID      uint64
	workGroup  *kernels.WorkGroup
	dst        messaging.RemotePort
}

// cuProcessor implements modeling.EventProcessor for the emulation
// ComputeUnit. It receives protocol.MapWGReq messages, functionally emulates
// the work-groups, and responds with protocol.WGCompletionMsg.
type cuProcessor struct {
	// TODO(akita5): state purity — these runtime structures hold pointers
	// (kernels, wavefronts, decoded instructions) and cannot live in the
	// pure component State. They are not checkpointable.
	queuedWGs          []protocol.MapWGReq
	wfs                map[*kernels.WorkGroup][]*Wavefront
	instCache          map[uint64]*insts.Inst
	pendingCompletions []pendingWGCompletion
}

// Process reacts to a wakeup: it collects newly arrived MapWGReqs, runs the
// emulation pass when due, and sends out matured work-group completions.
func (p *cuProcessor) Process(comp *Comp, now timing.VTimeInPicoSec) bool {
	progress := false

	progress = p.collectMapWGReqs(comp, now) || progress
	progress = p.runEmulation(comp, now) || progress
	progress = p.completeWorkGroups(comp, now) || progress

	return progress
}

func (p *cuProcessor) collectMapWGReqs(
	comp *Comp,
	now timing.VTimeInPicoSec,
) bool {
	port := comp.GetPortByName(DispatchPortName)
	state := &comp.State
	progress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			break
		}

		req := msg.(protocol.MapWGReq)

		if state.NextEmulationAt <= now {
			state.NextEmulationAt = ceilToSecond(now)
			comp.ScheduleWakeAt(state.NextEmulationAt)
		}

		p.queuedWGs = append(p.queuedWGs, req)
		p.wfs[req.WorkGroup] = make([]*Wavefront, 0, 64)

		progress = true
	}

	return progress
}

func (p *cuProcessor) runEmulation(
	comp *Comp,
	now timing.VTimeInPicoSec,
) bool {
	if len(p.queuedWGs) == 0 {
		return false
	}

	if now < comp.State.NextEmulationAt {
		comp.ScheduleWakeAt(comp.State.NextEmulationAt)
		return false
	}

	for len(p.queuedWGs) > 0 {
		req := p.queuedWGs[0]
		p.queuedWGs = p.queuedWGs[1:]
		p.runWG(comp, req, now)
	}

	return true
}

func (p *cuProcessor) runWG(
	comp *Comp,
	req protocol.MapWGReq,
	now timing.VTimeInPicoSec,
) {
	wg := req.WorkGroup
	p.initWfs(wg, req)

	alu := comp.Resources().ALU
	for !p.isAllWfCompleted(wg) {
		for _, wf := range p.wfs[wg] {
			alu.SetLDS(wf.LDS)
			p.runWfUntilBarrier(comp, wf)
		}
		p.resolveBarrier(wg)
	}

	completeAt := comp.Spec().Freq.NextTick(now)
	p.pendingCompletions = append(p.pendingCompletions, pendingWGCompletion{
		completeAt: completeAt,
		reqID:      req.ID,
		workGroup:  wg,
		dst:        req.Src,
	})
	comp.ScheduleWakeAt(completeAt)
}

func (p *cuProcessor) initWfs(
	wg *kernels.WorkGroup,
	req protocol.MapWGReq,
) {
	lds := p.initLDS(req)

	for _, wf := range wg.Wavefronts {
		managedWf := NewWavefront(wf)
		managedWf.LDS = lds
		managedWf.pid = req.PID
		p.wfs[wg] = append(p.wfs[wg], managedWf)
	}

	for _, managedWf := range p.wfs[wg] {
		p.initWfRegs(managedWf)
	}
}

func (p *cuProcessor) initLDS(req protocol.MapWGReq) []byte {
	ldsSize := req.WorkGroup.Packet.GroupSegmentSize
	lds := make([]byte, ldsSize)
	return lds
}

//nolint:funlen,gocyclo
func (p *cuProcessor) initWfRegs(wf *Wavefront) {
	co := wf.CodeObject
	pkt := wf.Packet

	wf.SetPC(pkt.KernelObject + co.KernelCodeEntryByteOffset)
	wf.SetEXEC(wf.InitExecMask)

	SGPRPtr := 0
	if co.EnableSgprPrivateSegmentBuffer {
		SGPRPtr += 16
	}

	if co.EnableSgprDispatchPtr {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], wf.PacketAddress)
		SGPRPtr += 8
	}

	if co.EnableSgprQueuePtr {
		// Note: QueuePtr is not currently supported. For V5+ kernels, the kernel
		// descriptor flags may be incorrect. We do NOT reserve space, as the
		// kernel may not actually use this register.
	}

	if co.EnableSgprKernargSegmentPtr {
		binary.LittleEndian.PutUint64(wf.SRegFile[SGPRPtr:SGPRPtr+8], pkt.KernargAddress)
		SGPRPtr += 8
	}

	if co.EnableSgprDispatchID {
		log.Printf("EnableSgprDispatchID is not supported")
		//fmt.Printf("s%d SGPRDispatchID\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprFlatScratchInit {
		log.Printf("EnableSgprFlatScratchInit is not supported")
		//fmt.Printf("s%d SGPRFlatScratchInit\n", SGPRPtr/4)
		SGPRPtr += 8
	}

	if co.EnableSgprPrivateSegmentSize {
		// Note: PrivateSegmentSize is not currently supported. For V5+ kernels,
		// the kernel descriptor flags may be incorrect. We do NOT reserve space.
	}

	if co.EnableSgprGridWorkgroupCountX {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeX+uint32(pkt.WorkgroupSizeX)-1)/uint32(pkt.WorkgroupSizeX))
		//fmt.Printf("s%d WorkGroupCountX\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkgroupCountY {
		binary.LittleEndian.PutUint32(wf.SRegFile[SGPRPtr:SGPRPtr+4],
			(pkt.GridSizeY+uint32(pkt.WorkgroupSizeY)-1)/uint32(pkt.WorkgroupSizeY))
		//fmt.Printf("s%d WorkGroupCountY\n", SGPRPtr/4)
		SGPRPtr += 4
	}

	if co.EnableSgprGridWorkgroupCountZ {
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
		// SGPRPtr += 4
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Printf("EnableSgprPrivateSegmentSize is not supported")
		// SGPRPtr += 4
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Printf("EnableSgprPrivateSegentWaveByteOffset is not supported")
		// SGPRPtr += 4
	}

	var x, y, z int
	for i := wf.FirstWiFlatID; i < wf.FirstWiFlatID+64; i++ {
		z = i / (wf.WG.SizeX * wf.WG.SizeY)
		y = i % (wf.WG.SizeX * wf.WG.SizeY) / wf.WG.SizeX
		x = i % (wf.WG.SizeX * wf.WG.SizeY) % wf.WG.SizeX
		laneID := i - wf.FirstWiFlatID

		if co.Version == insts.CodeObjectV5 {
			// For V5 code objects (gfx942/CDNA3), pack work-item IDs into v0
			// as: v0 = (z << 20) | (y << 10) | x
			packed := uint32(x) | (uint32(y) << 10) | (uint32(z) << 20)
			wf.WriteReg(insts.VReg(0), 1, laneID, insts.Uint32ToBytes(packed))
		} else {
			// For V2/V3 code objects (GCN3), use separate registers
			wf.WriteReg(insts.VReg(0), 1, laneID, insts.Uint32ToBytes(uint32(x)))

			if co.EnableVgprWorkItemID() > 0 {
				wf.WriteReg(insts.VReg(1), 1, laneID, insts.Uint32ToBytes(uint32(y)))
			}

			if co.EnableVgprWorkItemID() > 1 {
				wf.WriteReg(insts.VReg(2), 1, laneID, insts.Uint32ToBytes(uint32(z)))
			}
		}
	}
}

func (p *cuProcessor) isAllWfCompleted(wg *kernels.WorkGroup) bool {
	for _, wf := range p.wfs[wg] {
		if !wf.Completed {
			return false
		}
	}
	return true
}

func (p *cuProcessor) runWfUntilBarrier(comp *Comp, wf *Wavefront) {
	if wf.Completed {
		return
	}

	resources := comp.Resources()

	for {
		pc := wf.PC()
		inst, ok := p.instCache[pc]
		if !ok {
			instBuf := resources.StorageAccessor.Read(wf.pid, pc, 8)
			var err error
			inst, err = resources.Decoder.Decode(instBuf)
			if err != nil {
				log.Panicf("Failed to decode instruction at PC=0x%x: %v (bytes: %x)", pc, err, instBuf)
			}
			p.instCache[pc] = inst
		}
		wf.inst = inst

		wf.SetPC(wf.PC() + uint64(inst.ByteSize))

		if inst.FormatType == insts.SOPP && inst.Opcode == 10 { // S_BARRIER
			wf.AtBarrier = true
			p.logInst(comp, wf, inst)
			break
		}

		if inst.FormatType == insts.SOPP && inst.Opcode == 1 { // S_ENDPGM
			wf.Completed = true
			p.logInst(comp, wf, inst)
			break
		}

		p.executeInst(comp.Resources().ALU, wf)
		p.logInst(comp, wf, inst)
	}
}

func (p *cuProcessor) logInst(comp *Comp, wf *Wavefront, inst *insts.Inst) {
	ctx := hooking.HookCtx{
		Domain: comp,
		Item:   wf,
		Detail: inst,
	}
	comp.InvokeHook(ctx)
}

func (p *cuProcessor) executeInst(alu ALU, wf *Wavefront) {
	alu.Run(wf)
}

func (p *cuProcessor) resolveBarrier(wg *kernels.WorkGroup) {
	if p.isAllWfCompleted(wg) {
		return
	}

	for _, wf := range p.wfs[wg] {
		if !wf.AtBarrier {
			log.Panic("not all wavefronts at barrier")
		}
		wf.AtBarrier = false
	}
}

// completeWorkGroups retires the pending work-group completions that have
// matured. Once all the in-flight work-groups have completed, it sends a
// single WGCompletionMsg that acknowledges all of them.
func (p *cuProcessor) completeWorkGroups(
	comp *Comp,
	now timing.VTimeInPicoSec,
) bool {
	state := &comp.State
	progress := false
	remaining := make([]pendingWGCompletion, 0, len(p.pendingCompletions))

	for _, pc := range p.pendingCompletions {
		if pc.completeAt > now {
			remaining = append(remaining, pc)
			comp.ScheduleWakeAt(pc.completeAt)
			continue
		}

		delete(p.wfs, pc.workGroup)
		if !containsID(state.FinishedMapWGReqIDs, pc.reqID) {
			state.FinishedMapWGReqIDs =
				append(state.FinishedMapWGReqIDs, pc.reqID)
		}
		state.CompletionDst = pc.dst

		progress = true
	}

	p.pendingCompletions = remaining

	return p.sendWGCompletionMsg(comp) || progress
}

func (p *cuProcessor) sendWGCompletionMsg(comp *Comp) bool {
	state := &comp.State

	if len(state.FinishedMapWGReqIDs) == 0 || len(p.wfs) != 0 {
		return false
	}

	port := comp.GetPortByName(DispatchPortName)
	if !port.CanSend() {
		// NotifyPortFree will wake the component up to retry.
		return false
	}

	msg := protocol.WGCompletionMsg{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: port.AsRemote(),
			Dst: state.CompletionDst,
		},
		RspToIDs: state.FinishedMapWGReqIDs,
	}
	port.Send(msg)

	state.FinishedMapWGReqIDs = nil

	return true
}

func containsID(ids []uint64, id uint64) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}
