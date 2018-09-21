package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/mem"
)

// A VectorMemoryUnit performs Scalar operations
type VectorMemoryUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	coalescer          Coalescer

	SendBuf     []mem.AccessReq
	SendBufSize int

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// NewVectorMemoryUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	coalescer Coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu

	u.scratchpadPreparer = scratchpadPreparer
	u.coalescer = coalescer

	u.SendBufSize = 256
	u.SendBuf = make([]mem.AccessReq, 0, u.SendBufSize)

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(wave *Wavefront, now akita.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toRead.inst, "ReadStart"})
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
		u.cu.InvokeHook(u.toRead, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toRead.inst, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toRead.inst, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
		return true
	}
	return false
}

func (u *VectorMemoryUnit) runExecStage(now akita.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	if u.toWrite == nil {
		inst := u.toExec.Inst()
		switch inst.FormatType {
		case insts.FLAT:
			u.executeFlatInsts(now)
		default:
			log.Panicf("running inst %s in vector memory unit is not supported", inst.String(nil))
		}

		u.cu.InvokeHook(u.toExec, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toExec.inst, "ExecEnd"})
		u.cu.InvokeHook(u.toExec, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toExec.inst, "WaitMem"})

		//u.toWrite = u.toExec
		u.toExec.State = WfReady
		u.toExec = nil
		return true
	}
	return false
}

func (u *VectorMemoryUnit) executeFlatInsts(now akita.VTimeInSec) {
	u.toExec.OutstandingVectorMemAccess++
	u.toExec.OutstandingScalarMemAccess++
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 16: // FLAT_LOAD_BYTE
		u.executeFlatLoad(1, now)
	case 20: // FLAT_LOAD_DWORD
		u.executeFlatLoad(4, now)
	case 23: // FLAT_LOAD_DWORDx4
		u.executeFlatLoad(16, now)
	case 28: // FLAT_STORE_DWORD
		u.executeFlatStore(4, now)
	case 31: // FLAT_STORE_DWORDx4
		u.executeFlatStore(16, now)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}
}

func (u *VectorMemoryUnit) executeFlatLoad(byteSizePerLane int, now akita.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	coalescedAddrs := u.coalesceAddress(sp.ADDR[:], byteSizePerLane)
	u.bufferDataLoadRequest(coalescedAddrs, sp.ADDR, byteSizePerLane/4, now)
}

func (u *VectorMemoryUnit) executeFlatStore(byteSizePerLane int, now akita.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	coalescedAddrs := u.coalesceAddress(sp.ADDR[:], byteSizePerLane)
	u.bufferDataStoreRequest(coalescedAddrs, sp.ADDR, sp.DATA, byteSizePerLane/4, now)
}

func (u *VectorMemoryUnit) coalesceAddress(
	addresses []uint64,
	byteSizePerLane int,
) []CoalescedAccess {
	return u.coalescer.Coalesce(addresses, byteSizePerLane)
}

func (u *VectorMemoryUnit) bufferDataLoadRequest(
	coalescedAddrs []CoalescedAccess,
	preCoalescedAddrs [64]uint64,
	registerCount int,
	now akita.VTimeInSec,
) {
	for i, addr := range coalescedAddrs {
		info := new(VectorMemAccessInfo)
		info.Inst = u.toExec.inst
		info.Wavefront = u.toExec
		info.DstVGPR = u.toExec.inst.Dst.Register
		info.Lanes = addr.LaneIDs
		info.LaneAddrOffsets = addr.LaneAddrOffset
		info.RegisterCount = registerCount

		lowModule := u.cu.VectorMemModules.Find(addr.Addr)
		req := mem.NewReadReq(now, u.cu.ToVectorMem, lowModule, addr.Addr, addr.Size)
		info.Read = req
		if i == len(coalescedAddrs)-1 {
			req.IsLastInWave = true
		}
		u.cu.inFlightVectorMemAccess = append(u.cu.inFlightVectorMemAccess, info)
		u.SendBuf = append(u.SendBuf, req)
	}
}

func (u *VectorMemoryUnit) bufferDataStoreRequest(
	coalescedAddrs []CoalescedAccess,
	preCoalescedAddrs [64]uint64,
	data [256]uint32,
	registerCount int,
	now akita.VTimeInSec,
) {
	for i, addr := range preCoalescedAddrs {
		info := new(VectorMemAccessInfo)
		info.Wavefront = u.toExec
		info.Inst = u.toExec.inst
		info.DstVGPR = u.toExec.inst.Dst.Register

		lowModule := u.cu.VectorMemModules.Find(addr)
		req := mem.NewWriteReq(now, u.cu.ToVectorMem, lowModule, addr)
		info.Write = req
		req.Address = addr
		if i == len(preCoalescedAddrs)-1 {
			req.IsLastInWave = true
		}

		for j := 0; j < registerCount; j++ {
			req.Data = append(req.Data, insts.Uint32ToBytes(data[i*4+j])...)
		}
		u.SendBuf = append(u.SendBuf, req)
		u.cu.inFlightVectorMemAccess = append(
			u.cu.inFlightVectorMemAccess, info)
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
