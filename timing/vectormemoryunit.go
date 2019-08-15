package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
<<<<<<< HEAD
=======
	"gitlab.com/akita/util/tracing"
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
)

// A VectorMemoryUnit performs Scalar operations
type VectorMemoryUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
<<<<<<< HEAD
	coalescer          Coalescer
=======
	coalescer          coalescer
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	SendBuf     []mem.AccessReq
	SendBufSize int

	toRead  *wavefront.Wavefront
	toExec  *wavefront.Wavefront
	toWrite *wavefront.Wavefront

	AddrCoalescingLatency   int
	AddrCoalescingCycleLeft int

	isIdle bool
}

// NewVectorMemoryUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
<<<<<<< HEAD
	coalescer Coalescer,
=======
	coalescer coalescer,
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu

	u.scratchpadPreparer = scratchpadPreparer
	u.coalescer = coalescer

	u.SendBufSize = 256
	u.SendBuf = make([]mem.AccessReq, 0, u.SendBufSize)

	u.AddrCoalescingLatency = 40

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(wave *wavefront.Wavefront, now akita.VTimeInSec) {
	u.toRead = wave
	u.cu.logInstStageTask(now, wave.DynamicInst(), "read", false)
}

// IsIdle moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toExec == nil) && (u.toWrite == nil) && (len(u.SendBuf) == 0)
	return u.isIdle
}

// Run executes three pipeline stages that are controlled by the VectorMemoryUnit
func (u *VectorMemoryUnit) Run(now akita.VTimeInSec) bool {
	madeProgress := false
	madeProgress = madeProgress || u.sendRequest(now)
	madeProgress = madeProgress || u.runExecStage(now)
	madeProgress = madeProgress || u.runReadStage(now)
	return madeProgress
}

func (u *VectorMemoryUnit) runReadStage(now akita.VTimeInSec) bool {
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)

		u.cu.logInstStageTask(now, u.toRead.DynamicInst(), "read", true)
		u.cu.logInstStageTask(now, u.toRead.DynamicInst(), "exec", false)

		u.toExec = u.toRead
		u.toRead = nil
		u.AddrCoalescingCycleLeft = u.AddrCoalescingLatency
		return true
	}
	return false
}

func (u *VectorMemoryUnit) runExecStage(now akita.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	if u.AddrCoalescingCycleLeft > 0 {
		u.AddrCoalescingCycleLeft--
		return true
	}

	if u.toWrite == nil {
		inst := u.toExec.Inst()
		switch inst.FormatType {
		case insts.FLAT:
			u.executeFlatInsts(now)
		default:
			log.Panicf("running inst %s in vector memory unit is not supported", inst.String(nil))
		}

		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "exec", true)
		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "mem", false)

		//u.toWrite = u.toExec

		u.cu.UpdatePCAndSetReady(u.toExec)
		u.toExec = nil

		return true
	}
	return false
}

func (u *VectorMemoryUnit) executeFlatInsts(now akita.VTimeInSec) {
<<<<<<< HEAD
	u.toExec.OutstandingVectorMemAccess++
	u.toExec.OutstandingScalarMemAccess++
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 16: // FLAT_LOAD_BYTE
		u.executeFlatLoad(1, now)
	case 18: // FLAT_LOAD_USHORT
		u.executeFlatLoad(2, now)
	case 20: // FLAT_LOAD_DWORD
		u.executeFlatLoad(4, now)
	case 23: // FLAT_LOAD_DWORDx4
		u.executeFlatLoad(16, now)
	case 28: // FLAT_STORE_DWORD
		u.executeFlatStore(4, now)
	case 31: // FLAT_STORE_DWORDx4
		u.executeFlatStore(16, now)
=======

	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 16, 17, 18, 19, 20, 21, 22, 23: // FLAT_LOAD_BYTE
		u.executeFlatLoad(now)
	case 24, 25, 26, 27, 28, 29, 30, 31:
		u.executeFlatStore(now)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}
}

<<<<<<< HEAD
func (u *VectorMemoryUnit) executeFlatLoad(byteSizePerLane int, now akita.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	//preCoalescedAddress := make([]uint64, 0, 64)
	//for i := uint64(0); i < 64; i++ {
	//	if sp.EXEC&(1<<i) > 0 {
	//		preCoalescedAddress = append(preCoalescedAddress, sp.ADDR[i])
	//	}
	//}
	coalescedAddrs := u.coalescer.Coalesce(sp.ADDR[:], sp.EXEC, byteSizePerLane)
	u.bufferDataLoadRequest(coalescedAddrs, sp.ADDR, byteSizePerLane/4, now)
}

func (u *VectorMemoryUnit) executeFlatStore(byteSizePerLane int, now akita.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	coalescedAddrs := u.coalescer.Coalesce(sp.ADDR[:], sp.EXEC, byteSizePerLane)
	u.bufferDataStoreRequest(coalescedAddrs, sp.ADDR, sp.DATA, sp.EXEC, byteSizePerLane/4, now)
}

func (u *VectorMemoryUnit) bufferDataLoadRequest(
	coalescedAddrs []CoalescedAccess,
	preCoalescedAddrs [64]uint64,
	registerCount int,
	now akita.VTimeInSec,
) {
	for i, addr := range coalescedAddrs {
		info := new(VectorMemAccessInfo)
		info.Inst = u.toExec.DynamicInst()
		info.Wavefront = u.toExec
		info.DstVGPR = u.toExec.Inst().Dst.Register
		info.Lanes = addr.LaneIDs
		info.LaneAddrOffsets = addr.LaneAddrOffset
		info.RegisterCount = registerCount

		lowModule := u.cu.VectorMemModules.Find(addr.Addr)
		req := mem.NewReadReq(now,
			u.cu.ToVectorMem, lowModule,
			addr.Addr, addr.Size)
		req.PID = u.toExec.PID()

		info.Read = req
		if i == len(coalescedAddrs)-1 {
			req.IsLastInWave = true
		}
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, info)
		u.SendBuf = append(u.SendBuf, req)
	}
}

func (u *VectorMemoryUnit) bufferDataStoreRequest(
	coalescedAddrs []CoalescedAccess,
	preCoalescedAddrs [64]uint64,
	data [256]uint32,
	execMask uint64,
	registerCount int,
	now akita.VTimeInSec,
) {
	lastLaneIndex := 0
	for i := 0; i < 64; i++ {
		if execMask&(1<<uint64(i)) > 0 {
			lastLaneIndex = i
		}
	}

	for i, addr := range preCoalescedAddrs {
		if execMask&(1<<uint64(i)) == 0 {
			continue
		}

		info := new(VectorMemAccessInfo)
		info.Wavefront = u.toExec
		info.Inst = u.toExec.DynamicInst()
		info.DstVGPR = u.toExec.Inst().Dst.Register

		lowModule := u.cu.VectorMemModules.Find(addr)
		req := mem.NewWriteReq(now, u.cu.ToVectorMem, lowModule, addr)
		info.Write = req
		req.PID = u.toExec.PID()
		if i == lastLaneIndex {
			req.IsLastInWave = true
		}

		for j := 0; j < registerCount; j++ {
			req.Data = append(req.Data, insts.Uint32ToBytes(data[i*4+j])...)
		}
		u.SendBuf = append(u.SendBuf, req)
		u.cu.InFlightVectorMemAccess = append(
			u.cu.InFlightVectorMemAccess, info)
=======
func (u *VectorMemoryUnit) executeFlatLoad(
	now akita.VTimeInSec,
) {
	transactions := u.coalescer.generateMemTransactions(u.toExec)

	if len(transactions) == 0 {
		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "mem", true)
		u.cu.logInstTask(now, u.toExec, u.toExec.DynamicInst(), true)
		return
	}

	u.toExec.OutstandingVectorMemAccess++
	u.toExec.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i == len(transactions)-1 {
			t.Read.IsLastInWave = true
		}
		lowModule := u.cu.VectorMemModules.Find(t.Read.Address)
		t.Read.SetDst(lowModule)
		t.Read.SetSrc(u.cu.ToVectorMem)
		t.Read.PID = u.toExec.PID()
		u.SendBuf = append(u.SendBuf, t.Read)
		tracing.TraceReqInitiate(t.Read, now, u.cu, u.toExec.DynamicInst().ID)
	}
}

func (u *VectorMemoryUnit) executeFlatStore(
	now akita.VTimeInSec,
) {
	transactions := u.coalescer.generateMemTransactions(u.toExec)

	if len(transactions) == 0 {
		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "mem", true)
		u.cu.logInstTask(now, u.toExec, u.toExec.DynamicInst(), true)
		return
	}

	u.toExec.OutstandingVectorMemAccess++
	u.toExec.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i == len(transactions)-1 {
			t.Write.IsLastInWave = true
		}
		lowModule := u.cu.VectorMemModules.Find(t.Write.Address)
		t.Write.SetDst(lowModule)
		t.Write.SetSrc(u.cu.ToVectorMem)
		t.Write.PID = u.toExec.PID()
		u.SendBuf = append(u.SendBuf, t.Write)
		tracing.TraceReqInitiate(t.Write, now, u.cu, u.toExec.DynamicInst().ID)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	}
}

func (u *VectorMemoryUnit) sendRequest(now akita.VTimeInSec) bool {
	madeProgress := false

	if len(u.SendBuf) > 0 {
		req := u.SendBuf[0]
		req.SetSendTime(now)
		err := u.cu.ToVectorMem.Send(req)
		if err == nil {
			u.SendBuf = u.SendBuf[1:]
			madeProgress = true
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) Flush() {
	u.SendBuf = nil
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
<<<<<<< HEAD
=======
	u.SendBuf = nil
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
}
