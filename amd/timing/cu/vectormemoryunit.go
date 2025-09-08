package cu

import (
	"log"

	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

type vectorMemInst struct {
	wavefront *wavefront.Wavefront
}

func (i vectorMemInst) TaskID() string {
	return i.wavefront.DynamicInst().ID
}

// A VectorMemoryUnit is the block in a compute unit that can performs vector
// memory operations.
type VectorMemoryUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	coalescer          coalescer

	numInstInFlight         uint64
	numTransactionInFlight  uint64
	maxInstructionsInFlight uint64

	instructionPipeline           pipelining.Pipeline
	postInstructionPipelineBuffer sim.Buffer
	transactionsWaiting           []VectorMemAccessInfo
	transactionPipeline           pipelining.Pipeline
	postTransactionPipelineBuffer sim.Buffer

	isIdle bool
}

// NewVectorMemoryUnit creates a new Vector Memory Unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	coalescer coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu

	u.scratchpadPreparer = scratchpadPreparer
	u.coalescer = coalescer

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.instructionPipeline.CanAccept()
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(
	wave *wavefront.Wavefront,
) {
	u.instructionPipeline.Accept(vectorMemInst{wavefront: wave})
	u.numInstInFlight++
}

// IsIdle moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) IsIdle() bool {
	u.isIdle = (u.numInstInFlight == 0) && (u.numTransactionInFlight == 0)
	return u.isIdle
}

// Run executes three pipeline stages that are controlled by the
// VectorMemoryUnit
func (u *VectorMemoryUnit) Run() bool {
	madeProgress := false

	sendProgress := u.sendRequest()
	transPipelineProgress := u.transactionPipeline.Tick()
	instToTransProgress := u.instToTransaction()
	instPipelineProgress := u.instructionPipeline.Tick()

	madeProgress = sendProgress || madeProgress
	madeProgress = transPipelineProgress || madeProgress
	madeProgress = instToTransProgress || madeProgress
	madeProgress = instPipelineProgress || madeProgress
	return madeProgress
}

func (u *VectorMemoryUnit) instToTransaction() bool {
	if len(u.transactionsWaiting) > 0 {
		return u.insertTransactionToPipeline()
	}

	return u.execute()
}

func (u *VectorMemoryUnit) insertTransactionToPipeline() bool {
	if !u.transactionPipeline.CanAccept() {
		return false
	}

	u.transactionPipeline.Accept(u.transactionsWaiting[0])
	u.transactionsWaiting = u.transactionsWaiting[1:]

	return true
}

func (u *VectorMemoryUnit) execute() (madeProgress bool) {
	item := u.postInstructionPipelineBuffer.Peek()
	if item == nil {
		return false
	}

	wave := item.(vectorMemInst).wavefront
	inst := wave.Inst()

	switch inst.FormatType {
	case insts.FLAT:
		ok := u.executeFlatInsts(wave)
		if !ok {
			return false
		}
	default:
		log.Panicf("running inst %s in vector memory unit is not supported", inst.String(nil))
	}

	u.postInstructionPipelineBuffer.Pop()
	u.cu.UpdatePCAndSetReady(wave)
	u.numInstInFlight--

	return true
}

func (u *VectorMemoryUnit) executeFlatInsts(
	wavefront *wavefront.Wavefront,
) bool {
	inst := wavefront.DynamicInst()
	switch inst.Opcode {
	case 16, 17, 18, 19, 20, 21, 22, 23: // FLAT_LOAD_BYTE
		return u.executeFlatLoad(wavefront)
	case 24, 25, 26, 27, 28, 29, 30, 31:
		return u.executeFlatStore(wavefront)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}

	panic("never")
}

func (u *VectorMemoryUnit) executeFlatLoad(
	wave *wavefront.Wavefront,
) bool {
	u.scratchpadPreparer.Prepare(wave, wave)
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.logInstTask(
			wave,
			wave.DynamicInst(),
			true,
		)
		return true
	}

	if len(transactions)+len(u.cu.InFlightVectorMemAccess) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Read.CanWaitForCoalesce = true
		}

		lowModule := u.cu.VectorMemModules.Find(t.Read.Address)
		t.Read.Dst = lowModule
		t.Read.Src = u.cu.ToVectorMem.AsRemote()
		t.Read.PID = wave.PID()
		u.transactionsWaiting = append(u.transactionsWaiting, t)
	}

	return true
}

func (u *VectorMemoryUnit) executeFlatStore(
	wave *wavefront.Wavefront,
) bool {
	u.scratchpadPreparer.Prepare(wave, wave)
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.logInstTask(
			wave,
			wave.DynamicInst(),
			true,
		)
		return true
	}

	if len(transactions)+len(u.cu.InFlightVectorMemAccess) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Write.CanWaitForCoalesce = true
		}
		lowModule := u.cu.VectorMemModules.Find(t.Write.Address)
		t.Write.Dst = lowModule
		t.Write.Src = u.cu.ToVectorMem.AsRemote()
		t.Write.PID = wave.PID()
		u.transactionsWaiting = append(u.transactionsWaiting, t)
	}

	return true
}

func (u *VectorMemoryUnit) sendRequest() bool {
	item := u.postTransactionPipelineBuffer.Peek()
	if item == nil {
		return false
	}

	var req sim.Msg
	info := item.(VectorMemAccessInfo)
	if info.Read != nil {
		req = info.Read
	} else {
		req = info.Write
	}

	err := u.cu.ToVectorMem.Send(req)
	if err == nil {
		u.postTransactionPipelineBuffer.Pop()
		u.numTransactionInFlight--

		tracing.TraceReqInitiate(req, u.cu, info.Inst.ID)

		return true
	}

	return false
}

// Flush flushes
func (u *VectorMemoryUnit) Flush() {
	u.instructionPipeline.Clear()
	u.transactionPipeline.Clear()
	u.postInstructionPipelineBuffer.Clear()
	u.postTransactionPipelineBuffer.Clear()
	u.transactionsWaiting = nil
	u.numInstInFlight = 0
	u.numTransactionInFlight = 0
}
