package smsp

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"

	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// SMSPController models one SM sub-partition: a warp scheduler that issues
// trace instructions into per-opcode pipelines and sends global memory
// accesses to its L1 vector cache.
type SMSPController struct {
	*Comp
}

type Spec struct {
	ToSM                   messaging.RemotePort `json:"to_sm"`
	ToVectorMem            messaging.RemotePort `json:"to_vector_mem"`
	Freq                   timing.Freq          `json:"freq"`
	Log2CacheLineSize      uint64               `json:"log2_cache_line_size"`
	MemResponseHandleWidth uint64               `json:"mem_response_handle_width"`
}

type State struct {
	Scheduler            SMSPSWarpScheduler
	ResourcePool         ResourcePool
	UnsentMemPipelines   []PipelineRef
	InflightMemPipelines map[uint64]PipelineRef
	FinishedWarps        []trace.WarpRef
	InstsCount           uint64
}

type Resources struct{ Traces *trace.Store }
type Ports struct {
	ToSM        messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.smsp"`
	ToVectorMem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Middlewares struct{ Controller *SMSPController }

func (m *Middlewares) Initialize(c *Comp) {
	m.Controller = &SMSPController{Comp: c}
}
func (c *SMSPController) Handle(timing.Event) bool { return c.Tick() }

// Tick advances the SMSP by one cycle.
func (s *SMSPController) Tick() bool {
	madeProgress := false

	s.State.ResourcePool.Reset()
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.sendMemRequests() || madeProgress
	madeProgress = s.processMemRsp() || madeProgress

	return madeProgress
}

func (s *SMSPController) processSMInput() bool {
	msg, ok := s.Ports.ToSM.PeekIncoming()
	if !ok {
		return false
	}

	switch msg := msg.Payload.(type) {
	case message.SMToSMSPMsg:
		s.State.Scheduler.insertWarps(msg.WarpList, s.Resources.Traces)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from SM", s.Name(), msg))
	}

	s.Ports.ToSM.RetrieveIncoming()

	return true
}

func (s *SMSPController) releasePipelineSrcRegs(pipe *PipelineInstance) bool {
	if pipe.SrcRegsReleased {
		return false
	}

	s.State.Scheduler.warp(pipe.WarpID).Scoreboard.MarkSrcRegsConsumed(pipe.SrcRegs)
	pipe.MarkSrcRegsReleased()

	return true
}

func (s *SMSPController) releasePipelineDstRegs(pipe *PipelineInstance) bool {
	if pipe.DstRegsReleased {
		return false
	}

	s.State.Scheduler.warp(pipe.WarpID).Scoreboard.MarkDstRegsCompleted(pipe.DstRegs)
	pipe.MarkDstRegsReleased()

	return true
}

func (s *SMSPController) processMemRsp() bool {
	madeProgress := false

	for range max(s.Spec.MemResponseHandleWidth, 1) {
		msg, ok := s.Ports.ToVectorMem.PeekIncoming()
		if !ok {
			break
		}

		switch msg.Payload.(type) {
		case memprotocol.DataReadyRsp:
			s.completeMemPipeline(msg.RspTo)
		case memprotocol.WriteDoneRsp:
			s.completeMemPipeline(msg.RspTo)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from memory",
				s.Name(), msg))
		}

		s.Ports.ToVectorMem.RetrieveIncoming()

		madeProgress = true
	}

	return madeProgress
}

func (s *SMSPController) completeMemPipeline(reqID uint64) {
	ref, found := s.State.InflightMemPipelines[reqID]
	if !found {
		panic(fmt.Sprintf("%s: memory response to unknown request %d",
			s.Name(), reqID))
	}

	delete(s.State.InflightMemPipelines, reqID)
	pipe := s.State.Scheduler.pipeline(ref)

	s.releasePipelineSrcRegs(pipe)
	s.releasePipelineDstRegs(pipe)
	pipe.MarkMemoryResponseReady()
	s.State.Scheduler.warp(pipe.WarpID).updateStatus()
}

// skipMaskedControlOps retires fully-masked BRA/EXIT instructions without
// sending them through a pipeline.
func (s *SMSPController) skipMaskedControlOps() bool {
	madeProgress := false

	for wi := range s.State.Scheduler.WarpUnits {
		wu := &s.State.Scheduler.WarpUnits[wi]
		for wu.HasMoreToIssue() {
			inst := wu.NextInstruction(s.Resources.Traces)
			if inst.Mask != 0 || (inst.OpCode != "BRA" && inst.OpCode != "EXIT") {
				break
			}

			wu.NextIssueInstIndex++
			wu.UnfinishedInstsCount--
			s.State.InstsCount++
			madeProgress = true
		}

		wu.updateStatus()
	}

	return madeProgress
}

func (s *SMSPController) launchIssuedPipelines(issued []*IssueDecision) {
	for _, decision := range issued {
		pipe := decision.Pipeline
		s.State.InstsCount++

		if pipe.IsMemoryPipeline() {
			pipe.MarkMemoryRequestSent()
			s.releasePipelineSrcRegs(pipe)
			s.State.UnsentMemPipelines = append(s.State.UnsentMemPipelines, pipe.Ref())
		}

		decision.WarpUnit.updateStatus()
	}
}

func (s *SMSPController) tickInFlightPipelines() bool {
	madeProgress := false

	for wi := range s.State.Scheduler.WarpUnits {
		wu := &s.State.Scheduler.WarpUnits[wi]
		remaining := make([]PipelineInstance, 0, len(wu.InFlightPipelines))

		for pi := range wu.InFlightPipelines {
			pipe := &wu.InFlightPipelines[pi]
			if !pipe.Done && !pipe.IsMemoryPipeline() {
				madeProgress = pipe.Tick() || madeProgress

				if pipe.ReadyToReleaseSrcRegs() {
					madeProgress = s.releasePipelineSrcRegs(pipe) || madeProgress
				}
			}

			if pipe.Done {
				s.releasePipelineSrcRegs(pipe)
				s.releasePipelineDstRegs(pipe)
				wu.UnfinishedInstsCount--
				madeProgress = true

				continue
			}
			remaining = append(remaining, *pipe)
		}

		wu.InFlightPipelines = remaining
		wu.updateStatus()
	}

	return madeProgress
}

func (s *SMSPController) collectFinishedWarps() bool {
	madeProgress := false
	remaining := s.State.Scheduler.WarpUnits[:0]

	for wi := range s.State.Scheduler.WarpUnits {
		wu := &s.State.Scheduler.WarpUnits[wi]
		if wu.UnfinishedInstsCount == 0 && len(wu.InFlightPipelines) == 0 {
			s.State.FinishedWarps = append(s.State.FinishedWarps, wu.Trace)
			madeProgress = true

			continue
		}
		remaining = append(remaining, *wu)
	}

	s.State.Scheduler.WarpUnits = remaining

	return madeProgress
}

func (s *SMSPController) run() bool {
	if s.State.Scheduler.isEmpty() {
		return false
	}

	madeProgress := s.skipMaskedControlOps()

	issued := s.State.Scheduler.issueWarps(&s.State.ResourcePool, s.Resources.Traces)
	if len(issued) > 0 {
		s.launchIssuedPipelines(issued)

		madeProgress = true
	}

	madeProgress = s.tickInFlightPipelines() || madeProgress
	madeProgress = s.collectFinishedWarps() || madeProgress

	return madeProgress
}

func (s *SMSPController) reportFinishedWarps() bool {
	if len(s.State.FinishedWarps) == 0 || !s.Ports.ToSM.CanSend() {
		return false
	}

	msg := messaging.Msg{ID: s.NewID(), Src: s.Ports.ToSM.AsRemote(), Dst: s.Spec.ToSM, Payload: message.SMSPToSMMsg{
		WarpFinished: true,
		SMSPID:       s.Name(),
		Warp:         s.State.FinishedWarps[0],
	}}
	s.Ports.ToSM.Send(msg)
	s.State.FinishedWarps = s.State.FinishedWarps[1:]

	return true
}

func (s *SMSPController) sendMemRequests() bool {
	madeProgress := false

	for len(s.State.UnsentMemPipelines) > 0 && s.Ports.ToVectorMem.CanSend() {
		ref := s.State.UnsentMemPipelines[0]
		pipe := s.State.Scheduler.pipeline(ref)

		var msg messaging.Msg

		switch pipe.CurrentStage().Def.Name {
		case "MemoryPipeRead":
			msg = s.readReq(&s.Resources.Traces.Warp(s.State.Scheduler.warp(pipe.WarpID).Trace).Instructions[pipe.InstIndex])
		case "MemoryPipeWrite":
			msg = s.writeReq(&s.Resources.Traces.Warp(s.State.Scheduler.warp(pipe.WarpID).Trace).Instructions[pipe.InstIndex])
		default:
			panic("unknown memory pipeline stage " + pipe.CurrentStage().Def.Name)
		}

		s.Ports.ToVectorMem.Send(msg)
		s.State.InflightMemPipelines[msg.ID] = ref
		s.State.UnsentMemPipelines = s.State.UnsentMemPipelines[1:]
		madeProgress = true
	}

	return madeProgress
}

// accessRange clips an access so that it does not cross a cache line.
func (s *SMSPController) accessRange(addr, byteSize uint64) (uint64, uint64) {
	lineSize := uint64(1) << s.Spec.Log2CacheLineSize
	byteSize = max(byteSize, 1)

	if byteSize > lineSize {
		byteSize = lineSize
	}

	if addr%lineSize+byteSize > lineSize {
		addr = addr/lineSize*lineSize + lineSize - byteSize
	}

	return addr, byteSize
}

func (s *SMSPController) readReq(inst *trace.InstructionTrace) messaging.Msg {
	addr, byteSize := s.accessRange(
		inst.MemAddress, uint64(max(inst.MemAddressSuffix1, inst.MemWidth)))

	return messaging.Msg{
		ID:  s.NewID(),
		Src: s.Ports.ToVectorMem.AsRemote(),
		Dst: s.Spec.ToVectorMem,
		Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: byteSize,
			PID:            1,
		},
	}
}

func (s *SMSPController) writeReq(inst *trace.InstructionTrace) messaging.Msg {
	addr, byteSize := s.accessRange(inst.MemAddress, 4)

	return messaging.Msg{
		ID:  s.NewID(),
		Src: s.Ports.ToVectorMem.AsRemote(),
		Dst: s.Spec.ToVectorMem,
		Payload: memprotocol.WriteReq{
			Address: addr,
			Data:    make([]byte, byteSize),
			PID:     1,
		},
	}
}

// GetTotalInstsCount returns the number of instructions issued or skipped
// so far.
func (s *SMSPController) GetTotalInstsCount() uint64 {
	return s.State.InstsCount
}

// Status reports how much work the SMSP holds: resident warps, memory
// requests waiting to be sent, and memory requests waiting for a response.
func (s *SMSPController) Status() (warps, unsentMem, inflightMem int) {
	return len(s.State.Scheduler.WarpUnits), len(s.State.UnsentMemPipelines),
		len(s.State.InflightMemPipelines)
}

func (s *State) Initialize(c *Comp) {
	if c.Resources.Traces == nil {
		panic("NVIDIA component requires a shared trace store")
	}
	if c.Spec.ToSM == "" || c.Spec.ToVectorMem == "" {
		panic(fmt.Sprintf("SMSP requires SM and memory destinations: to_sm=%q to_vector_mem=%q",
			c.Spec.ToSM, c.Spec.ToVectorMem))
	}
	s.Scheduler = *NewSMSPScheduler()
	s.ResourcePool = *NewH100SMSPResourcePool()
	s.InflightMemPipelines = make(map[uint64]PipelineRef)
}
