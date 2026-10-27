package cu

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type InstructionState struct {
	ID, PC uint64
	Raw    []byte
	// Owner is the generation-bearing local wave slot. ExecutionPending spans
	// issue through execution completion, including the final memory response.
	Owner            uint64
	ExecutionPending bool
}
type WaveState struct {
	ID            uint64
	Generation    uint32
	Group, Index  int
	StopFetch     bool
	Execution     wavefront.ExecutionState
	HasScoreboard bool
	Scoreboard    Scoreboard
}
type WorkGroupState struct {
	Occupied bool
	Raw      kernels.WorkGroupValue
	Request  messaging.Msg
	LDS      []byte
	Waves    []uint64
}
type FetchState struct {
	Req               messaging.Msg
	WF, Address, Task uint64
}
type ScalarAccessState struct {
	Req      messaging.Msg
	WF, Inst uint64
	Register insts.RegType
}
type LaneState struct {
	Lane     int
	Register insts.RegType
	Count    int
	Offset   uint64
}
type VectorAccessState struct {
	Read, Write messaging.Msg
	WF, Inst    uint64
	Lanes       []LaneState
}
type DecoderState struct {
	ToDecode      []uint64
	Decoded, Idle bool
	Tasks         map[uint64]uint64
	Done          map[uint64]bool
}
type StageState struct {
	Read, Exec, Write uint64
	Cycles            int
	Idle              bool
}
type ScalarUnitState struct {
	Stages StageState
	Reads  []messaging.Msg
	// Tasks holds scalar address/read-request issue subtasks until admission.
	Tasks map[uint64]uint64
}
type SchedulerState struct {
	Internal, Barrier    []uint64
	NoProgress, LastSIMD int
}
type VectorUnitState struct {
	Instructions, Transactions uint64
	Stall                      int
	Idle                       bool
	// IssueTasks holds coalescing subtasks until the first transaction is sent.
	IssueTasks           map[uint64]uint64
	Waiting              []VectorAccessState
	InstructionsPipeline queueing.Pipeline[vectorMemInst]
	InstructionsOutput   queueing.Buffer[vectorMemInst]
	TransactionsPipeline queueing.Pipeline[uint64]
	TransactionsOutput   queueing.Buffer[uint64]
}
type SIMDSlotState struct {
	WF     uint64
	Cycles int
	Task   uint64
}
type SIMDState struct {
	Exec   uint64
	Cycles int
	Task   uint64
	Idle   bool
	Slots  []SIMDSlotState
}

func (s *State) Initialize(c *Comp) {
	*s = State{
		InstTasks: make(map[uint64]uint64), WFTasks: make(map[uint64]uint64),
		WGTasks: make(map[uint64]uint64), RequestTasks: make(map[uint64]uint64),
		Groups:       make([]WorkGroupState, numWfPools*c.Spec.WfPoolSize),
		Waves:        make([]WaveState, numWfPools*c.Spec.WfPoolSize),
		Instructions: make(map[uint64]InstructionState),
		WFTimes:      make(map[uint64]timing.VTimeInPicoSec), Pools: make([][]uint64, numWfPools),
		SRegisters: make([]byte, c.Spec.SGPRCount*4), VRegisters: make([][]byte, c.Spec.SIMDCount),
		Decoders: make([]DecoderState, 4), SIMDs: make([]SIMDState, c.Spec.SIMDCount),
	}
	for i := range s.VRegisters {
		s.VRegisters[i] = make([]byte, c.Spec.VGPRCounts[i]*4)
	}
	for i := range s.Decoders {
		s.Decoders[i].Tasks = make(map[uint64]uint64)
		s.Decoders[i].Done = make(map[uint64]bool)
	}
	s.ScalarUnit.Tasks = make(map[uint64]uint64)
	s.VectorUnit.IssueTasks = make(map[uint64]uint64)
	s.VectorUnit.InstructionsOutput = queueing.MakeBuffer[vectorMemInst](4 * c.Spec.SIMDCount)
	s.VectorUnit.InstructionsPipeline = queueing.MakePipeline[vectorMemInst](
		c.Spec.SIMDCount, c.Spec.VecMemInstPipelineStages)
	s.VectorUnit.TransactionsOutput = queueing.MakeBuffer[uint64](max(8, c.Spec.MemPipelineBufferSize))
	s.VectorUnit.TransactionsPipeline = queueing.MakePipeline[uint64](
		max(1, c.Spec.VecMemTransPipelineWidth), c.Spec.VecMemTransPipelineStages)
}
