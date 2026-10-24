package cu

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type cuCheckpoint struct{ cu *ComputeUnit }

func (r *cuCheckpoint) Name() string { return r.cu.comp.Name() + ".Runtime" }

type savedInst struct {
	ID, PC uint64
	Raw    []byte
}
type savedWave struct {
	ID            uint64
	Execution     wavefront.ExecutionState
	Inst, ToIssue uint64
	HasScoreboard bool
	Scoreboard    Scoreboard
}
type savedWG struct {
	Raw     kernels.WorkGroupValue
	Request messaging.Msg
	LDS     []byte
	Waves   []savedWave
}
type savedFetch struct {
	Req               messaging.Msg
	WF, Address, Task uint64
}
type savedScalarAccess struct {
	Req      messaging.Msg
	WF, Inst uint64
	Register insts.RegType
}
type savedLane struct {
	Lane     int
	Register insts.RegType
	Count    int
	Offset   uint64
}
type savedVectorAccess struct {
	Read, Write messaging.Msg
	WF, Inst    uint64
	Lanes       []savedLane
}
type savedDecoder struct {
	ToDecode      []uint64
	Decoded, Idle bool
	Tasks         map[uint64]uint64
	Done          map[uint64]bool
}
type savedStages struct {
	Read, Exec, Write uint64
	Cycles            int
	Idle              bool
}
type savedScalarUnit struct {
	Stages savedStages
	Reads  []messaging.Msg
	Tasks  map[uint64]uint64
}
type savedScheduler struct {
	Internal, Barrier    []uint64
	NoProgress, LastSIMD int
	Paused               bool
}
type savedVectorUnit struct {
	Instructions, Transactions, MaxInstructions uint64
	Stall                                       int
	Idle                                        bool
	IssueTasks                                  map[uint64]uint64
	Waiting                                     []savedVectorAccess
	InstructionsPipeline                        queueing.Pipeline[vectorMemInst]
	InstructionsOutput                          queueing.Buffer[vectorMemInst]
	TransactionsPipeline                        queueing.Pipeline[uint64]
	TransactionsOutput                          queueing.Buffer[uint64]
}
type savedCU struct {
	Groups                     map[uint64]savedWG
	Instructions               map[uint64]savedInst
	Pools                      [][]uint64
	InstFetch, ShadowInstFetch []savedFetch
	Scalar, ShadowScalar       []savedScalarAccess
	Vector, ShadowVector       []savedVectorAccess
	WFTimes                    map[uint64]timing.VTimeInPicoSec
	SRegisters                 []byte
	VRegisters                 [][]byte
	Scheduler                  savedScheduler
	Decoders                   []savedDecoder
	Branch, LDS                savedStages
	ScalarUnit                 savedScalarUnit
	VectorUnit                 savedVectorUnit
}

func wfID(w *wavefront.Wavefront) uint64 {
	if w == nil {
		return 0
	}
	return w.UID
}
func instID(i *wavefront.Inst) uint64 {
	if i == nil {
		return 0
	}
	return i.ID
}
func waveIDs(waves []*wavefront.Wavefront) []uint64 {
	ids := make([]uint64, len(waves))
	for i, w := range waves {
		ids[i] = wfID(w)
	}
	return ids
}
func saveDecoder(d *DecodeUnit) savedDecoder {
	return savedDecoder{waveIDs(d.toDecode), d.decoded, d.isIdle, d.decodeTaskIDs, d.decodeDone}
}
func saveStages(read, exec, write *wavefront.Wavefront, cycles int, idle bool) savedStages {
	return savedStages{wfID(read), wfID(exec), wfID(write), cycles, idle}
}
func saveVector(v VectorMemAccessInfo) savedVectorAccess {
	s := savedVectorAccess{Read: v.Read, Write: v.Write, WF: wfID(v.Wavefront), Inst: instID(v.Inst)}
	for _, l := range v.laneInfo {
		s.Lanes = append(s.Lanes, savedLane{l.laneID, l.reg.RegType, l.regCount, l.addrOffsetInCacheLine})
	}
	return s
}

func (r *cuCheckpoint) SaveCheckpoint(w io.Writer) error {
	cu := r.cu
	s := savedCU{Groups: make(map[uint64]savedWG), Instructions: make(map[uint64]savedInst), WFTimes: cu.wftime}

	if err := s.saveGroups(cu); err != nil {
		return err
	}
	for _, p := range cu.WfPools {
		s.Pools = append(s.Pools, waveIDs(p.wfs))
	}

	s.InstFetch = s.saveFetches(cu.InFlightInstFetch)
	s.ShadowInstFetch = s.saveFetches(cu.shadowInFlightInstFetch)
	var err error
	if s.Scalar, err = s.saveScalars(cu.InFlightScalarMemAccess); err != nil {
		return err
	}
	if s.ShadowScalar, err = s.saveScalars(cu.shadowInFlightScalarMemAccess); err != nil {
		return err
	}
	if s.Vector, err = s.saveVectors(cu.InFlightVectorMemAccess); err != nil {
		return err
	}
	if s.ShadowVector, err = s.saveVectors(cu.shadowInFlightVectorMemAccess); err != nil {
		return err
	}
	if err := s.saveUnits(cu); err != nil {
		return err
	}
	return modeling.WriteCheckpoint(w, cu.comp.Spec, s, nil)
}

func (r *cuCheckpoint) LoadCheckpoint(reader io.Reader) error {
	cu := r.cu
	var s savedCU
	if err := modeling.ReadCheckpoint(reader, cu.comp.Spec, &s, nil); err != nil {
		return err
	}
	instructions, waves, err := s.restoreGraph(cu)
	if err != nil {
		return err
	}
	graph := &restoredCUGraph{waves: waves, instructions: instructions}
	if len(s.Pools) != len(cu.WfPools) || len(s.VRegisters) != len(cu.VRegFile) || len(s.Decoders) != 4 {
		return fmt.Errorf("cu: runtime geometry mismatch")
	}
	if len(s.SRegisters) != len(cu.SRegFile.(*SimpleRegisterFile).storage) {
		return fmt.Errorf("cu: scalar register size mismatch")
	}
	for i, stored := range s.VRegisters {
		if len(stored) != len(cu.VRegFile[i].(*SimpleRegisterFile).storage) {
			return fmt.Errorf("cu: vector register size mismatch")
		}
	}
	cu.wftime = s.WFTimes
	for i, ids := range s.Pools {
		cu.WfPools[i].wfs = graph.wfs(ids)
	}
	cu.InFlightInstFetch = graph.fetches(s.InstFetch)
	cu.shadowInFlightInstFetch = graph.fetches(s.ShadowInstFetch)
	cu.InFlightScalarMemAccess = graph.scalars(s.Scalar)
	cu.shadowInFlightScalarMemAccess = graph.scalars(s.ShadowScalar)
	cu.InFlightVectorMemAccess = graph.vectors(s.Vector)
	cu.shadowInFlightVectorMemAccess = graph.vectors(s.ShadowVector)
	cu.SRegFile.(*SimpleRegisterFile).storage = s.SRegisters
	for i, stored := range s.VRegisters {
		cu.VRegFile[i].(*SimpleRegisterFile).storage = stored
	}
	s.restoreUnits(cu, graph)
	s.restoreVectorUnit(cu, graph)
	return graph.err
}

type savedSIMDSlot struct {
	WF     uint64
	Cycles int
	Task   uint64
}
type savedSIMD struct {
	Exec   uint64
	Cycles int
	Task   uint64
	Idle   bool
	Slots  []savedSIMDSlot
}

// SaveCheckpoint records the SIMD pipeline independently so the SIMD also has
// a registered identity for scoped task observers. Its parent CU runtime is
// registered first and restores the shared wavefront graph before this loads.
func (u *SIMDUnit) SaveCheckpoint(w io.Writer) error {
	s := savedSIMD{Exec: wfID(u.toExec), Cycles: u.cycleLeft, Task: u.execTaskID, Idle: u.isIdle}
	for _, p := range u.pipelineSlots {
		s.Slots = append(s.Slots, savedSIMDSlot{wfID(p.wf), p.cycleLeft, p.taskID})
	}
	return modeling.WriteCheckpoint(w, u.cu.comp.Spec, s, nil)
}
func (u *SIMDUnit) LoadCheckpoint(r io.Reader) error {
	var s savedSIMD
	if err := modeling.ReadCheckpoint(r, u.cu.comp.Spec, &s, nil); err != nil {
		return err
	}
	u.toExec = nil
	if s.Exec != 0 {
		u.toExec = u.cu.findWavefront(s.Exec)
		if u.toExec == nil {
			return fmt.Errorf("cu: missing SIMD wavefront %d", s.Exec)
		}
	}
	u.cycleLeft = s.Cycles
	u.execTaskID = s.Task
	u.isIdle = s.Idle
	u.pipelineSlots = nil
	for _, p := range s.Slots {
		wf := u.cu.findWavefront(p.WF)
		if wf == nil {
			return fmt.Errorf("cu: missing SIMD slot wavefront %d", p.WF)
		}
		u.pipelineSlots = append(u.pipelineSlots, &simdPipelineSlot{wf, p.Cycles, p.Task})
	}
	return nil
}

func (s *savedCU) rememberInstruction(i *wavefront.Inst) error {
	if i == nil {
		return nil
	}
	if len(i.Raw) == 0 {
		return fmt.Errorf("cu: instruction %d has no encoding", i.ID)
	}
	s.Instructions[i.ID] = savedInst{i.ID, i.PC, i.Raw}
	return nil
}

func (s *savedCU) saveGroups(cu *ComputeUnit) error {
	for id, wg := range cu.workGroups {
		g := savedWG{Raw: kernels.WorkGroupSnapshot(wg.WorkGroup), Request: wg.MapReq, LDS: wg.LDS}
		for _, wf := range wg.Wfs {
			if err := s.rememberInstruction(wf.DynamicInst()); err != nil {
				return err
			}
			if err := s.rememberInstruction(wf.InstToIssue); err != nil {
				return err
			}
			v := savedWave{
				ID:        wf.UID,
				Execution: wf.SnapshotExecution(),
				Inst:      instID(wf.DynamicInst()),
				ToIssue:   instID(wf.InstToIssue),
			}
			if wf.ScoreboardData != nil {
				v.HasScoreboard = true
				v.Scoreboard = *wf.ScoreboardData.(*Scoreboard)
			}
			g.Waves = append(g.Waves, v)
		}
		s.Groups[id] = g
	}
	return nil
}
func (s *savedCU) saveFetches(in []*InstFetchReqInfo) []savedFetch {
	out := make([]savedFetch, len(in))
	for i, v := range in {
		out[i] = savedFetch{v.Req, wfID(v.Wavefront), v.Address, v.FetchTaskID}
	}
	return out
}
func (s *savedCU) saveScalars(in []*ScalarMemAccessInfo) ([]savedScalarAccess, error) {
	out := make([]savedScalarAccess, len(in))
	for i, v := range in {
		if err := s.rememberInstruction(v.Inst); err != nil {
			return nil, err
		}
		out[i] = savedScalarAccess{v.Req, wfID(v.Wavefront), instID(v.Inst), v.DstSGPR.RegType}
	}
	return out, nil
}
func (s *savedCU) saveVectors(in []VectorMemAccessInfo) ([]savedVectorAccess, error) {
	out := make([]savedVectorAccess, len(in))
	for i, v := range in {
		if err := s.rememberInstruction(v.Inst); err != nil {
			return nil, err
		}
		out[i] = saveVector(v)
	}
	return out, nil
}

func (s *savedCU) restoreGraph(cu *ComputeUnit) (map[uint64]*wavefront.Inst, map[uint64]*wavefront.Wavefront, error) {
	instructions := make(map[uint64]*wavefront.Inst)
	for id, v := range s.Instructions {
		raw, err := cu.Decoder.Decode(v.Raw)
		if err != nil {
			return nil, nil, fmt.Errorf("cu: decode saved instruction %d: %w", id, err)
		}
		raw.PC = v.PC
		i := wavefront.NewInst(id, raw)
		i.Raw = v.Raw
		instructions[id] = i
	}
	waves := make(map[uint64]*wavefront.Wavefront)
	groups := make(map[uint64]*wavefront.WorkGroup)
	for id, g := range s.Groups {
		raw, err := g.Raw.Restore()
		if err != nil {
			return nil, nil, err
		}
		wg := wavefront.NewWorkGroup(raw, g.Request)
		wg.LDS = g.LDS
		groups[id] = wg
		if len(g.Waves) != len(raw.Wavefronts) {
			return nil, nil, fmt.Errorf("cu: wavefront count mismatch")
		}
		for j, v := range g.Waves {
			if v.ID != raw.Wavefronts[j].UID {
				return nil, nil, fmt.Errorf("cu: wavefront identity mismatch")
			}
			wf := wavefront.NewWavefront(raw.Wavefronts[j])
			wf.WG = wg
			wf.RestoreExecution(v.Execution)
			wf.SetDynamicInst(instructions[v.Inst])
			wf.InstToIssue = instructions[v.ToIssue]
			if v.Inst != 0 && wf.DynamicInst() == nil || v.ToIssue != 0 && wf.InstToIssue == nil {
				return nil, nil, fmt.Errorf("cu: missing saved instruction")
			}
			if v.HasScoreboard {
				sb := v.Scoreboard
				wf.ScoreboardData = &sb
			}
			wf.RegAccessor = &CURegFileAccessor{CU: cu, WF: wf}
			wg.Wfs = append(wg.Wfs, wf)
			waves[v.ID] = wf
		}
	}
	cu.workGroups = groups
	return instructions, waves, nil
}

type restoredCUGraph struct {
	waves        map[uint64]*wavefront.Wavefront
	instructions map[uint64]*wavefront.Inst
	err          error
}

func (g *restoredCUGraph) wf(id uint64) *wavefront.Wavefront {
	v := g.waves[id]
	if id != 0 && v == nil {
		g.err = fmt.Errorf("cu: missing wavefront %d", id)
	}
	return v
}
func (g *restoredCUGraph) inst(id uint64) *wavefront.Inst {
	v := g.instructions[id]
	if id != 0 && v == nil {
		g.err = fmt.Errorf("cu: missing instruction %d", id)
	}
	return v
}
func (g *restoredCUGraph) wfs(ids []uint64) []*wavefront.Wavefront {
	out := make([]*wavefront.Wavefront, len(ids))
	for i, id := range ids {
		out[i] = g.wf(id)
	}
	return out
}
func (g *restoredCUGraph) reg(id insts.RegType) *insts.Reg {
	v := insts.Regs[id]
	if v == nil {
		g.err = fmt.Errorf("cu: missing register %d", id)
	}
	return v
}
func (g *restoredCUGraph) fetches(in []savedFetch) []*InstFetchReqInfo {
	out := make([]*InstFetchReqInfo, len(in))
	for i, v := range in {
		out[i] = &InstFetchReqInfo{v.Req, g.wf(v.WF), v.Address, v.Task}
	}
	return out
}
func (g *restoredCUGraph) scalars(in []savedScalarAccess) []*ScalarMemAccessInfo {
	out := make([]*ScalarMemAccessInfo, len(in))
	for i, v := range in {
		out[i] = &ScalarMemAccessInfo{v.Req, g.wf(v.WF), g.reg(v.Register), g.inst(v.Inst)}
	}
	return out
}
func (g *restoredCUGraph) vectors(in []savedVectorAccess) []VectorMemAccessInfo {
	out := make([]VectorMemAccessInfo, len(in))
	for i, v := range in {
		x := VectorMemAccessInfo{Read: v.Read, Write: v.Write, Wavefront: g.wf(v.WF), Inst: g.inst(v.Inst)}
		for _, l := range v.Lanes {
			x.laneInfo = append(x.laneInfo, vectorMemAccessLaneInfo{l.Lane, g.reg(l.Register), l.Count, l.Offset})
		}
		out[i] = x
	}
	return out
}

func (s *savedCU) saveUnits(cu *ComputeUnit) error {
	s.SRegisters = cu.SRegFile.(*SimpleRegisterFile).storage
	for _, reg := range cu.VRegFile {
		s.VRegisters = append(s.VRegisters, reg.(*SimpleRegisterFile).storage)
	}
	sched := cu.Scheduler.(*SchedulerImpl)
	s.Scheduler = savedScheduler{
		waveIDs(sched.internalExecuting),
		waveIDs(sched.barrierBuffer),
		sched.cyclesNoProgress,
		sched.issueArbiter.(*IssueArbiter).lastSIMDID,
		sched.isPaused,
	}
	for _, d := range []SubComponent{cu.ScalarDecoder, cu.VectorDecoder, cu.LDSDecoder, cu.VectorMemDecoder} {
		s.Decoders = append(s.Decoders, saveDecoder(d.(*DecodeUnit)))
	}
	branch := cu.BranchUnit.(*BranchUnit)
	s.Branch = saveStages(branch.toRead, branch.toExec, branch.toWrite, 0, branch.isIdle)
	lds := cu.LDSUnit.(*LDSUnit)
	s.LDS = saveStages(lds.toRead, lds.toExec, lds.toWrite, lds.cycleLeft, lds.isIdle)
	scalar := cu.ScalarUnit.(*ScalarUnit)
	s.ScalarUnit = savedScalarUnit{
		saveStages(scalar.toRead, scalar.toExec, scalar.toWrite, 0, scalar.isIdle),
		scalar.readBuf,
		scalar.issueTaskIDs,
	}
	v := cu.VectorMemUnit.(*VectorMemoryUnit)
	waiting, err := s.saveVectors(v.transactionsWaiting)
	if err != nil {
		return err
	}
	s.VectorUnit = savedVectorUnit{
		v.numInstInFlight,
		v.numTransactionInFlight,
		v.maxInstructionsInFlight,
		v.coalescingStallRemaining,
		v.isIdle,
		v.issueTaskIDs,
		waiting,
		v.instructionPipeline,
		v.postInstructionPipelineBuffer,
		v.transactionPipeline,
		v.postTransactionPipelineBuffer,
	}
	return nil
}

func (s *savedCU) restoreUnits(cu *ComputeUnit, graph *restoredCUGraph) {
	sched := cu.Scheduler.(*SchedulerImpl)
	sched.internalExecuting = graph.wfs(s.Scheduler.Internal)
	sched.barrierBuffer = graph.wfs(s.Scheduler.Barrier)
	sched.cyclesNoProgress = s.Scheduler.NoProgress
	sched.isPaused = s.Scheduler.Paused
	sched.issueArbiter.(*IssueArbiter).lastSIMDID = s.Scheduler.LastSIMD
	for i, d := range []SubComponent{cu.ScalarDecoder, cu.VectorDecoder, cu.LDSDecoder, cu.VectorMemDecoder} {
		x := d.(*DecodeUnit)
		v := s.Decoders[i]
		x.toDecode = graph.wfs(v.ToDecode)
		x.decoded = v.Decoded
		x.isIdle = v.Idle
		x.decodeTaskIDs = v.Tasks
		x.decodeDone = v.Done
	}
	b := cu.BranchUnit.(*BranchUnit)
	b.toRead = graph.wf(s.Branch.Read)
	b.toExec = graph.wf(s.Branch.Exec)
	b.toWrite = graph.wf(s.Branch.Write)
	b.isIdle = s.Branch.Idle
	l := cu.LDSUnit.(*LDSUnit)
	l.toRead = graph.wf(s.LDS.Read)
	l.toExec = graph.wf(s.LDS.Exec)
	l.toWrite = graph.wf(s.LDS.Write)
	l.isIdle = s.LDS.Idle
	l.cycleLeft = s.LDS.Cycles
	sc := cu.ScalarUnit.(*ScalarUnit)
	ss := s.ScalarUnit
	sc.toRead = graph.wf(ss.Stages.Read)
	sc.toExec = graph.wf(ss.Stages.Exec)
	sc.toWrite = graph.wf(ss.Stages.Write)
	sc.isIdle = ss.Stages.Idle
	sc.readBuf = ss.Reads
	sc.issueTaskIDs = ss.Tasks
}

func (s *savedCU) restoreVectorUnit(cu *ComputeUnit, graph *restoredCUGraph) {
	v := cu.VectorMemUnit.(*VectorMemoryUnit)
	vs := s.VectorUnit
	v.numInstInFlight = vs.Instructions
	v.numTransactionInFlight = vs.Transactions
	v.maxInstructionsInFlight = vs.MaxInstructions
	v.coalescingStallRemaining = vs.Stall
	v.isIdle = vs.Idle
	v.issueTaskIDs = vs.IssueTasks
	v.transactionsWaiting = graph.vectors(vs.Waiting)
	v.instructionPipeline = vs.InstructionsPipeline
	v.postInstructionPipelineBuffer = vs.InstructionsOutput
	v.transactionPipeline = vs.TransactionsPipeline
	v.postTransactionPipelineBuffer = vs.TransactionsOutput
}
