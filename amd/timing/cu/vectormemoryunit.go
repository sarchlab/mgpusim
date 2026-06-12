package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

type vectorMemInst struct {
	wavefront *wavefront.Wavefront
}

func (i vectorMemInst) TaskID() uint64 {
	return i.wavefront.DynamicInst().ID
}

// scratchPendingCompletion tracks a scratch memory operation that is
// in-flight. After cyclesRemaining reaches zero, the wavefront's
// OutstandingVectorMemAccess (and OutstandingScalarMemAccess) counters
// are decremented, allowing s_waitcnt to proceed.
type scratchPendingCompletion struct {
	wavefront       *wavefront.Wavefront
	inst            *wavefront.Inst
	cyclesRemaining int
}

// A VectorMemoryUnit is the block in a compute unit that can performs vector
// memory operations.
type VectorMemoryUnit struct {
	cu *ComputeUnit

	coalescer coalescer

	numInstInFlight         uint64
	numTransactionInFlight  uint64
	maxInstructionsInFlight uint64

	maxCoalescingPenalty     int
	coalescingStallRemaining int

	scratchLatency        int
	scratchStallRemaining int
	scratchExecuted       bool

	// pendingScratchCompletions tracks FLAT scratch operations that have
	// been functionally executed but are waiting for their modeled latency
	// to elapse before decrementing the outstanding access counters.
	pendingScratchCompletions []scratchPendingCompletion

	instructionPipeline           queueing.Pipeline[vectorMemInst]
	postInstructionPipelineBuffer queueing.Buffer[vectorMemInst]
	transactionsWaiting           []VectorMemAccessInfo
	transactionPipeline           queueing.Pipeline[VectorMemAccessInfo]
	postTransactionPipelineBuffer queueing.Buffer[VectorMemAccessInfo]

	isIdle bool
}

// NewVectorMemoryUnit creates a new Vector Memory Unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	coalescer coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu
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

// IsIdle checks if the unit has no work in progress.
func (u *VectorMemoryUnit) IsIdle() bool {
	u.isIdle = (u.numInstInFlight == 0) &&
		(u.numTransactionInFlight == 0) &&
		len(u.pendingScratchCompletions) == 0
	return u.isIdle
}

// Run executes pipeline stages controlled by the VectorMemoryUnit.
func (u *VectorMemoryUnit) Run() bool {
	madeProgress := false
	madeProgress = u.tickScratchCompletions() || madeProgress
	madeProgress = u.sendRequest() || madeProgress
	madeProgress = u.transactionPipeline.Tick(&u.postTransactionPipelineBuffer) || madeProgress
	madeProgress = u.instToTransaction() || madeProgress
	madeProgress = u.instructionPipeline.Tick(&u.postInstructionPipelineBuffer) || madeProgress
	return madeProgress
}

// tickScratchCompletions processes pending scratch completions. Each
// tick decrements the remaining cycles; when zero, the wavefront's
// outstanding access counters are decremented so s_waitcnt can proceed.
func (u *VectorMemoryUnit) tickScratchCompletions() bool {
	if len(u.pendingScratchCompletions) == 0 {
		return false
	}

	madeProgress := false
	remaining := u.pendingScratchCompletions[:0]

	for i := range u.pendingScratchCompletions {
		pc := &u.pendingScratchCompletions[i]
		pc.cyclesRemaining--
		if pc.cyclesRemaining <= 0 {
			// Scratch operation complete — decrement counters.
			pc.wavefront.OutstandingVectorMemAccess--
			pc.wavefront.OutstandingScalarMemAccess--
			u.cu.logInstTask(pc.wavefront, pc.inst, true)
			madeProgress = true
		} else {
			remaining = append(remaining, *pc)
			madeProgress = true // still counting down
		}
	}

	u.pendingScratchCompletions = remaining
	return madeProgress
}

func (u *VectorMemoryUnit) instToTransaction() bool {
	madeProgress := false

	madeProgress = u.insertTransactionToPipeline() || madeProgress

	// Process up to 8 instructions per cycle (matching widened inst pipeline width)
	for i := 0; i < 8; i++ {
		progress := u.execute()
		madeProgress = progress || madeProgress
		if !progress {
			break
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) insertTransactionToPipeline() bool {
	madeProgress := false

	if u.coalescingStallRemaining > 0 {
		u.coalescingStallRemaining--
		return false
	}

	for len(u.transactionsWaiting) > 0 {
		if !u.transactionPipeline.CanAccept() {
			break
		}

		txn := u.transactionsWaiting[0]
		u.transactionPipeline.Accept(txn)
		u.transactionsWaiting = u.transactionsWaiting[1:]
		madeProgress = true

		penalty := u.computeCoalescingPenalty(txn)
		if penalty > 0 {
			u.coalescingStallRemaining = penalty
			break
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) computeCoalescingPenalty(txn VectorMemAccessInfo) int {
	if txn.Read == nil {
		return 0
	}

	maxLanes := 64 / 4 // 64B cacheline / 4B elements = 16
	usedLanes := len(txn.laneInfo)

	if usedLanes >= maxLanes {
		return 0
	}

	wastedFraction := float64(maxLanes-usedLanes) / float64(maxLanes)
	penalty := int(wastedFraction * float64(u.maxCoalescingPenalty))

	return penalty
}

func (u *VectorMemoryUnit) execute() (madeProgress bool) {
	if u.postInstructionPipelineBuffer.Size() == 0 {
		return false
	}

	item := u.postInstructionPipelineBuffer.Elements[0]

	wave := item.wavefront
	inst := wave.Inst()
	switch inst.FormatType {
	case insts.FLAT:
		ok := u.executeFlatInsts(wave)
		if !ok {
			return false
		}
	default:
		log.Panicf("running inst %s in vector memory unit is not supported",
			insts.NewInstPrinter(nil).Print(inst))
	}

	u.postInstructionPipelineBuffer.Elements = u.postInstructionPipelineBuffer.Elements[1:]
	u.cu.UpdatePCAndSetReady(wave)
	u.numInstInFlight--

	return true
}

func (u *VectorMemoryUnit) executeFlatInsts(
	wavefront *wavefront.Wavefront,
) bool {
	inst := wavefront.DynamicInst()

	// SCRATCH segment (FlatSeg=1): per-wave private memory.
	// Route through the non-serializing scratch path.
	if inst.FlatSeg == 1 {
		return u.executeScratchFast(wavefront)
	}

	// FLAT SEG=0 scratch spill detection: when the kernel has scratch
	// allocated (PrivateSegmentByteSize > 0) and the accessed addresses
	// fall within the per-lane scratch range, these are compiler-generated
	// scratch spills (e.g., volatile variables). Route them through the
	// fast scratch path for correct per-wave isolation and avoid the
	// extremely slow global memory round-trip.
	if inst.Opcode >= 16 && inst.Opcode <= 31 {
		if u.isFlatScratchSpill(wavefront) {
			return u.executeScratchFast(wavefront)
		}
	}

	switch {
	case inst.Opcode >= 16 && inst.Opcode <= 23:
		return u.executeFlatLoad(wavefront)
	case inst.Opcode >= 24 && inst.Opcode <= 31:
		return u.executeFlatStore(wavefront)
	case inst.Opcode >= 64 && inst.Opcode <= 76:
		return u.executeFlatAtomic(wavefront)
	case inst.Opcode >= 96 && inst.Opcode <= 108:
		return u.executeFlatAtomic(wavefront)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}

	panic("never")
}

// isFlatScratchSpill returns true if the FLAT SEG=0 instruction appears
// to be a compiler-generated scratch spill. This is detected by checking:
//   - The wavefront has scratch memory allocated (ScratchPerLane > 0)
//   - The instruction uses SAddr mode (Addr.RegCount == 1)
//   - All active lane VGPR offsets fall within [0, ScratchPerLane)
//
// On real hardware, the flat scratch aperture distinguishes these.
// In the simulator, FLAT SEG=0 + SAddr instructions where the VGPR
// offset is within the scratch range are compiler-generated spills
// (e.g., from volatile variables).
func (u *VectorMemoryUnit) isFlatScratchSpill(
	wave *wavefront.Wavefront,
) bool {
	if wave.ScratchPerLane == 0 || wave.Scratch == nil {
		return false
	}

	inst := wave.Inst()

	// Must be SAddr mode (RegCount=1 means 32-bit VGPR + scalar base)
	if inst.Addr == nil || inst.Addr.RegCount != 1 {
		return false
	}

	exec := wave.EXEC()
	scratchLimit := uint64(wave.ScratchPerLane)

	// Check that VGPR offset for all active lanes is within scratch range.
	// We only check the VGPR component (not the scalar base) because the
	// scalar base is the kernel argument pointer for these spills.
	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		vgprOffset := wave.ReadOperand(inst.Addr, int(i)) & 0xFFFFFFFF
		if vgprOffset >= scratchLimit {
			return false
		}
	}

	return true
}

// executeScratchFast handles scratch memory operations without serializing
// the VectorMemoryUnit. It works for both:
//   - FlatSeg=1 (explicit SCRATCH segment)
//   - FlatSeg=0 scratch spills (detected by isFlatScratchSpill)
//
// It directly reads/writes the wavefront's per-wave scratch buffer,
// bypassing the global memory hierarchy. The VGPR value is used as
// the per-lane scratch offset (the SADDR scalar base is ignored for
// scratch routing).
//
// Outstanding access counters are incremented so s_waitcnt properly
// waits. A pending completion is scheduled to decrement the counters
// after scratchLatency cycles, modeling memory access latency.
func (u *VectorMemoryUnit) executeScratchFast(
	wave *wavefront.Wavefront,
) bool {
	inst := wave.Inst()
	exec := wave.EXEC()

	byteSize, isStore := flatScratchOpInfo(inst.Opcode)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		// Compute the per-lane scratch offset from the VGPR only.
		// For FlatSeg=1 with SADDR=OFF, only inst_offset is used.
		// For FlatSeg=0 spills with SAddr mode, we use the VGPR value
		// as the offset (ignoring the scalar base which is the kernel
		// arg pointer, not a scratch address).
		var localOffset uint64
		if inst.FlatSeg == 1 && inst.SAddr != nil && inst.SAddr.IntValue == 0x7F {
			// SADDR=OFF: VADDR is ignored, use inst_offset only
			localOffset = 0
		} else {
			localOffset = wave.ReadOperand(inst.Addr, i) & 0xFFFFFFFF
		}

		if inst.Offset0 != 0 {
			localOffset += uint64(int64(int32(inst.Offset0)))
		}

		scratchOff := uint64(i)*uint64(wave.ScratchPerLane) + localOffset

		if isStore {
			data := wave.ReadOperandBytes(inst.Data, i, byteSize)
			end := scratchOff + uint64(byteSize)
			if wave.Scratch != nil && end <= uint64(len(wave.Scratch)) {
				copy(wave.Scratch[scratchOff:end], data)
			}
		} else {
			buf := make([]byte, byteSize)
			end := scratchOff + uint64(byteSize)
			if wave.Scratch != nil && end <= uint64(len(wave.Scratch)) {
				copy(buf, wave.Scratch[scratchOff:end])
			}
			wave.WriteOperandBytes(inst.Dst, i, buf)
		}
	}

	// Increment outstanding access counters so s_waitcnt vmcnt(0) will
	// wait for the completion.
	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	// Schedule the completion after scratchLatency cycles.
	// Minimum 1 cycle to ensure the completion happens on a future tick.
	latency := u.scratchLatency
	if latency < 1 {
		latency = 1
	}

	u.pendingScratchCompletions = append(u.pendingScratchCompletions,
		scratchPendingCompletion{
			wavefront:       wave,
			inst:            wave.DynamicInst(),
			cyclesRemaining: latency,
		})

	return true
}

// flatScratchOpInfo returns the byte size and whether the opcode is a store
// for FLAT load/store instructions.
func flatScratchOpInfo(opcode insts.Opcode) (byteSize int, isStore bool) {
	switch opcode {
	case 0, 1, 16: // load_ubyte
		return 1, false
	case 2, 3, 17: // load_sbyte
		return 1, false
	case 4, 5, 18: // load_ushort
		return 2, false
	case 20: // load_dword
		return 4, false
	case 21: // load_dwordx2
		return 8, false
	case 22: // load_dwordx3
		return 12, false
	case 23: // load_dwordx4
		return 16, false
	case 24: // store_byte
		return 1, true
	case 26: // store_short
		return 2, true
	case 28: // store_dword
		return 4, true
	case 29: // store_dwordx2
		return 8, true
	case 30: // store_dwordx3
		return 12, true
	case 31: // store_dwordx4
		return 16, true
	default:
		log.Panicf("Opcode %d not supported for scratch", opcode)
		return 0, false
	}
}

// executeScratchInst is the legacy serializing scratch path.
// Retained for reference; new code uses executeScratchFast.
func (u *VectorMemoryUnit) executeScratchInst(
	wave *wavefront.Wavefront,
) bool {
	if u.scratchStallRemaining > 0 {
		u.scratchStallRemaining--
		return false
	}

	if !u.scratchExecuted {
		u.cu.scratchALU.SetScratch(wave.Scratch, wave.ScratchPerLane)
		u.cu.scratchALU.Run(wave)
		u.scratchExecuted = true

		if u.scratchLatency > 1 {
			u.scratchStallRemaining = u.scratchLatency - 1
			return false
		}
	}

	u.scratchExecuted = false
	u.cu.logInstTask(wave, wave.DynamicInst(), true)

	return true
}

func (u *VectorMemoryUnit) executeFlatLoad(
	wave *wavefront.Wavefront,
) bool {
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

func (u *VectorMemoryUnit) buildAtomicTransactions(
	wave *wavefront.Wavefront,
	accessSize uint64,
) []VectorMemAccessInfo {
	exec := wave.EXEC()
	inst := wave.DynamicInst()
	rawInst := wave.Inst()
	var transactions []VectorMemAccessInfo

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		addr := readFlatAddrForLane(wave, int(i))

		data := make([]byte, accessSize)
		// Read data operand from VGPRs for this lane
		if rawInst.Data != nil {
			dataReg := insts.NewVRegOperand(
				rawInst.Data.Register.RegIndex(),
				rawInst.Data.Register.RegIndex(),
				int(accessSize/4),
			)
			regData := wave.ReadOperandBytes(dataReg, int(i), int(accessSize))
			copy(data, regData)
		}

		writeReq := &mem.WriteReq{
			Address:   addr,
			Data:      data,
			DirtyMask: make([]bool, accessSize),
		}
		writeReq.ID = sim.GetIDGenerator().Generate()
		for j := uint64(0); j < accessSize; j++ {
			writeReq.DirtyMask[j] = true
		}

		txn := VectorMemAccessInfo{
			Write:     writeReq,
			Wavefront: wave,
			Inst:      inst,
		}
		transactions = append(transactions, txn)
	}

	return transactions
}

func (u *VectorMemoryUnit) executeFlatAtomic(
	wave *wavefront.Wavefront,
) bool {
	inst := wave.DynamicInst()

	// Determine access size: opcodes 64-76 are 32-bit (4 bytes),
	// opcodes 96-108 are 64-bit (8 bytes).
	accessSize := uint64(4)
	if inst.Opcode >= 96 {
		accessSize = 8
	}

	transactions := u.buildAtomicTransactions(wave, accessSize)

	if len(transactions) == 0 {
		u.cu.logInstTask(wave, inst, true)
		return true
	}

	if len(transactions)+len(u.cu.InFlightVectorMemAccess) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	// Pre-fill VDST so that a CAS comparison loop sees "success"
	// and exits. Without this, the VDST register is never updated and
	// the kernel may loop forever.
	//
	// This is a timing-model approximation: the functional result may be
	// incorrect, but it prevents infinite CAS retry loops that hang the
	// simulation.
	u.writeAtomicReturnToVDST(wave, accessSize)

	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(
			u.cu.InFlightVectorMemAccess, t)
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

// writeAtomicReturnToVDST writes a return value to the destination VGPR
// for flat_atomic instructions that have VDST (return-value) semantics.
func (u *VectorMemoryUnit) writeAtomicReturnToVDST(
	wave *wavefront.Wavefront,
	accessSize uint64,
) {
	rawInst := wave.Inst()
	if rawInst.Dst == nil || rawInst.Data == nil {
		return
	}

	inst := wave.DynamicInst()
	isCmpSwap := inst.Opcode == 65 || inst.Opcode == 97
	exec := wave.EXEC()

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		var regData []byte
		if isCmpSwap {
			dataRegIdx := rawInst.Data.Register.RegIndex() + int(accessSize/4)
			cmpReg := insts.NewVRegOperand(
				dataRegIdx, dataRegIdx, int(accessSize/4),
			)
			regData = wave.ReadOperandBytes(cmpReg, int(i), int(accessSize))
		} else {
			dataReg := insts.NewVRegOperand(
				rawInst.Data.Register.RegIndex(),
				rawInst.Data.Register.RegIndex(),
				int(accessSize/4),
			)
			regData = wave.ReadOperandBytes(dataReg, int(i), int(accessSize))
		}

		access := RegisterAccess{}
		access.WaveOffset = wave.VRegOffset
		access.Reg = rawInst.Dst.Register
		access.RegCount = int(accessSize / 4)
		access.LaneID = int(i)
		access.Data = regData
		u.cu.VRegFile[wave.SIMDID].Write(access)
	}
}

func (u *VectorMemoryUnit) sendRequest() bool {
	madeProgress := false
	for i := 0; i < 16; i++ {
		if u.postTransactionPipelineBuffer.Size() == 0 {
			break
		}

		info := u.postTransactionPipelineBuffer.Elements[0]

		var req sim.Msg
		if info.Read != nil {
			req = info.Read
		} else {
			req = info.Write
		}

		err := u.cu.ToVectorMem.Send(req)
		if err == nil {
			u.postTransactionPipelineBuffer.Elements = u.postTransactionPipelineBuffer.Elements[1:]
			u.numTransactionInFlight--
			tracing.TraceReqInitiate(req, u.cu, info.Inst.ID)
			madeProgress = true
		} else {
			break
		}
	}
	return madeProgress
}

// Flush flushes
func (u *VectorMemoryUnit) Flush() {
	u.instructionPipeline.Stages = nil
	u.transactionPipeline.Stages = nil
	u.postInstructionPipelineBuffer.Clear()
	u.postTransactionPipelineBuffer.Clear()
	u.transactionsWaiting = nil
	u.numInstInFlight = 0
	u.numTransactionInFlight = 0
	u.coalescingStallRemaining = 0
	u.scratchStallRemaining = 0
	u.scratchExecuted = false
	u.pendingScratchCompletions = nil
}
