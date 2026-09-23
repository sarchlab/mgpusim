package smsp

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// SMSPController models one SM sub-partition: a warp scheduler that issues
// trace instructions into per-opcode pipelines and sends global memory
// accesses to its L1 vector cache.
type SMSPController struct {
	*modeling.TickingComponent

	ID string

	toSM              messaging.Port
	toSMRemote        messaging.RemotePort
	toVectorMem       messaging.Port
	toVectorMemRemote messaging.RemotePort

	scheduler    *SMSPSWarpScheduler
	ResourcePool *ResourcePool

	// Memory pipelines whose request has not been sent yet.
	unsentMemPipelines []*PipelineInstance
	// Memory pipelines waiting for a response, keyed by request ID.
	inflightMemPipelines map[uint64]*PipelineInstance

	finishedWarps []*trace.WarpTrace

	instsCount             uint64
	log2CacheLineSize      uint64
	MemResponseHandleWidth uint64
}

// SetSMRemotePort sets the SM port that finished warps are reported to.
func (s *SMSPController) SetSMRemotePort(remote messaging.RemotePort) {
	s.toSMRemote = remote
}

// SetVectorMemRemote sets the L1 vector cache port that memory requests are
// sent to.
func (s *SMSPController) SetVectorMemRemote(remote messaging.RemotePort) {
	s.toVectorMemRemote = remote
}

// Tick advances the SMSP by one cycle.
func (s *SMSPController) Tick() bool {
	madeProgress := false

	s.ResourcePool.Reset()
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.sendMemRequests() || madeProgress
	madeProgress = s.processMemRsp() || madeProgress

	return madeProgress
}

func (s *SMSPController) processSMInput() bool {
	msg := s.toSM.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case message.SMToSMSPMsg:
		s.scheduler.insertWarps(msg.WarpList)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from SM", s.Name(), msg))
	}

	s.toSM.RetrieveIncoming()

	return true
}

func (s *SMSPController) releasePipelineSrcRegs(pipe *PipelineInstance) bool {
	if pipe.SrcRegsReleased {
		return false
	}

	pipe.Warp.Scoreboard.MarkSrcRegsConsumed(pipe.SrcRegs)
	pipe.MarkSrcRegsReleased()

	return true
}

func (s *SMSPController) releasePipelineDstRegs(pipe *PipelineInstance) bool {
	if pipe.DstRegsReleased {
		return false
	}

	pipe.Warp.Scoreboard.MarkDstRegsCompleted(pipe.DstRegs)
	pipe.MarkDstRegsReleased()

	return true
}

func (s *SMSPController) processMemRsp() bool {
	madeProgress := false

	for range max(s.MemResponseHandleWidth, 1) {
		msg := s.toVectorMem.PeekIncoming()
		if msg == nil {
			break
		}

		switch msg := msg.(type) {
		case memprotocol.DataReadyRsp:
			s.completeMemPipeline(msg.RspTo)
		case memprotocol.WriteDoneRsp:
			s.completeMemPipeline(msg.RspTo)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from memory",
				s.Name(), msg))
		}

		s.toVectorMem.RetrieveIncoming()

		madeProgress = true
	}

	return madeProgress
}

func (s *SMSPController) completeMemPipeline(reqID uint64) {
	pipe, found := s.inflightMemPipelines[reqID]
	if !found {
		panic(fmt.Sprintf("%s: memory response to unknown request %d",
			s.Name(), reqID))
	}

	delete(s.inflightMemPipelines, reqID)

	s.releasePipelineSrcRegs(pipe)
	s.releasePipelineDstRegs(pipe)
	pipe.MarkMemoryResponseReady()
	pipe.Warp.updateStatus()
}

// skipMaskedControlOps retires fully-masked BRA/EXIT instructions without
// sending them through a pipeline.
func (s *SMSPController) skipMaskedControlOps() bool {
	madeProgress := false

	for _, wu := range s.scheduler.warpUnitList {
		for wu.HasMoreToIssue() {
			inst := wu.NextInstruction()
			if inst.Mask != 0 || (inst.OpCode != "BRA" && inst.OpCode != "EXIT") {
				break
			}

			wu.nextIssueInstIndex++
			wu.unfinishedInstsCount--
			s.instsCount++
			madeProgress = true
		}

		wu.updateStatus()
	}

	return madeProgress
}

func (s *SMSPController) launchIssuedPipelines(issued []*IssueDecision) {
	for _, decision := range issued {
		pipe := decision.Pipeline
		s.instsCount++

		if pipe.IsMemoryPipeline() {
			pipe.MarkMemoryRequestSent()
			s.releasePipelineSrcRegs(pipe)
			s.unsentMemPipelines = append(s.unsentMemPipelines, pipe)
		}

		decision.WarpUnit.updateStatus()
	}
}

func (s *SMSPController) tickInFlightPipelines() bool {
	madeProgress := false

	for _, wu := range s.scheduler.warpUnitList {
		remaining := make([]*PipelineInstance, 0, len(wu.InFlightPipelines))

		for _, pipe := range wu.InFlightPipelines {
			if !pipe.Done && !pipe.IsMemoryPipeline() {
				madeProgress = pipe.Tick() || madeProgress

				if pipe.ReadyToReleaseSrcRegs() {
					madeProgress = s.releasePipelineSrcRegs(pipe) || madeProgress
				}
			}

			if pipe.Done {
				s.releasePipelineSrcRegs(pipe)
				s.releasePipelineDstRegs(pipe)
				wu.unfinishedInstsCount--
				madeProgress = true

				continue
			}

			remaining = append(remaining, pipe)
		}

		wu.InFlightPipelines = remaining
		wu.updateStatus()
	}

	return madeProgress
}

func (s *SMSPController) collectFinishedWarps() bool {
	madeProgress := false
	remaining := s.scheduler.warpUnitList[:0]

	for _, wu := range s.scheduler.warpUnitList {
		if wu.unfinishedInstsCount == 0 && len(wu.InFlightPipelines) == 0 {
			s.finishedWarps = append(s.finishedWarps, wu.warp)
			madeProgress = true

			continue
		}

		remaining = append(remaining, wu)
	}

	s.scheduler.warpUnitList = remaining

	return madeProgress
}

func (s *SMSPController) run() bool {
	if s.scheduler.isEmpty() {
		return false
	}

	madeProgress := s.skipMaskedControlOps()

	issued := s.scheduler.issueWarps(s.ResourcePool)
	if len(issued) > 0 {
		s.launchIssuedPipelines(issued)

		madeProgress = true
	}

	madeProgress = s.tickInFlightPipelines() || madeProgress
	madeProgress = s.collectFinishedWarps() || madeProgress

	return madeProgress
}

func (s *SMSPController) reportFinishedWarps() bool {
	if len(s.finishedWarps) == 0 || !s.toSM.CanSend() {
		return false
	}

	msg := message.SMSPToSMMsg{
		MsgMeta:      message.NewMeta(s.toSM.AsRemote(), s.toSMRemote),
		WarpFinished: true,
		SMSPID:       s.ID,
		Warp:         s.finishedWarps[0],
	}
	s.toSM.Send(msg)
	s.finishedWarps = s.finishedWarps[1:]

	return true
}

func (s *SMSPController) sendMemRequests() bool {
	madeProgress := false

	for len(s.unsentMemPipelines) > 0 && s.toVectorMem.CanSend() {
		pipe := s.unsentMemPipelines[0]

		var msg messaging.Msg

		switch pipe.CurrentStage().Def.Name {
		case "MemoryPipeRead":
			msg = s.readReq(pipe.Inst)
		case "MemoryPipeWrite":
			msg = s.writeReq(pipe.Inst)
		default:
			panic("unknown memory pipeline stage " + pipe.CurrentStage().Def.Name)
		}

		s.toVectorMem.Send(msg)
		s.inflightMemPipelines[msg.Meta().ID] = pipe
		s.unsentMemPipelines = s.unsentMemPipelines[1:]
		madeProgress = true
	}

	return madeProgress
}

// accessRange clips an access so that it does not cross a cache line.
func (s *SMSPController) accessRange(addr, byteSize uint64) (uint64, uint64) {
	lineSize := uint64(1) << s.log2CacheLineSize
	byteSize = max(byteSize, 1)

	if byteSize > lineSize {
		byteSize = lineSize
	}

	if addr%lineSize+byteSize > lineSize {
		addr = addr/lineSize*lineSize + lineSize - byteSize
	}

	return addr, byteSize
}

func (s *SMSPController) readReq(inst *trace.InstructionTrace) memprotocol.ReadReq {
	addr, byteSize := s.accessRange(
		inst.MemAddress, uint64(max(inst.MemAddressSuffix1, inst.MemWidth)))

	return memprotocol.ReadReq{
		MsgMeta:        message.NewMeta(s.toVectorMem.AsRemote(), s.toVectorMemRemote),
		Address:        addr,
		AccessByteSize: byteSize,
		PID:            1,
	}
}

func (s *SMSPController) writeReq(inst *trace.InstructionTrace) memprotocol.WriteReq {
	addr, byteSize := s.accessRange(inst.MemAddress, 4)

	return memprotocol.WriteReq{
		MsgMeta: message.NewMeta(s.toVectorMem.AsRemote(), s.toVectorMemRemote),
		Address: addr,
		Data:    make([]byte, byteSize),
		PID:     1,
	}
}

// GetTotalInstsCount returns the number of instructions issued or skipped
// so far.
func (s *SMSPController) GetTotalInstsCount() uint64 {
	return s.instsCount
}
