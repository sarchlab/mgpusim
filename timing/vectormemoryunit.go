package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A VectorMemoryUnit performs Scalar operations
type VectorMemoryUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// NewVectorMemoryUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ReadStart"})
}

// Run executes three pipeline stages that are controlled by the VectorMemoryUnit
func (u *VectorMemoryUnit) Run(now core.VTimeInSec) {
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *VectorMemoryUnit) runReadStage(now core.VTimeInSec) {
	if u.toRead == nil {
		return
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *VectorMemoryUnit) runExecStage(now core.VTimeInSec) {
	if u.toExec == nil {
		return
	}

	if u.toWrite == nil {

		inst := u.toExec.Inst()
		switch inst.FormatType {
		case insts.FLAT:
			u.executeFlatInsts(now)
		default:
			log.Panicf("running inst %f in vector memory unit is not supported", inst.String(nil))
		}

		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "ExecEnd"})
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "WaitMem"})

		//u.toWrite = u.toExec
		u.toExec.State = WfReady
		u.toExec = nil

	}
}

func (u *VectorMemoryUnit) executeFlatInsts(now core.VTimeInSec) {
	u.toExec.OutstandingVectorMemAccess++
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 20: // FLAT_LOAD_DWORD
		u.executeFlatLoad(4, now)
	case 28: // FLAT_STORE_DWORD
		u.executeFlatStore(4, now)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}
}

func (u *VectorMemoryUnit) executeFlatLoad(byteSizePerLane int, now core.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	coalescedAddrs := u.coalesceAddress(sp.ADDR)
	u.sendDataLoadRequest(coalescedAddrs, sp.ADDR, now)
}

func (u *VectorMemoryUnit) executeFlatStore(byteSizePerLane int, now core.VTimeInSec) {
	sp := u.toExec.Scratchpad().AsFlat()
	coalescedAddrs := u.coalesceAddress(sp.ADDR)
	u.sendDataStoreRequest(coalescedAddrs, sp.ADDR, sp.DATA, now)
}

func (u *VectorMemoryUnit) coalesceAddress(addresses [64]uint64) []uint64 {
	coalescedAddr := make([]uint64, 0)
	for i := 0; i < 64; i++ {
		addr := addresses[i]
		cacheLineId := addr / 64 * 64

		found := false
		for _, cAddr := range coalescedAddr {
			if cacheLineId == cAddr {
				found = true
				break
			}
		}

		if !found {
			coalescedAddr = append(coalescedAddr, cacheLineId)
		}
	}
	return coalescedAddr
}

func (u *VectorMemoryUnit) sendDataLoadRequest(
	coalescedAddrs []uint64,
	preCoalescedAddrs [64]uint64,
	now core.VTimeInSec,
) {
	info := new(MemAccessInfo)
	info.Action = MemAccessVectorDataLoad
	info.PreCoalescedAddrs = preCoalescedAddrs
	info.Wf = u.toExec
	info.Inst = info.Wf.inst
	info.Dst = info.Wf.inst.Dst.Register
	info.TotalReqs = len(coalescedAddrs)
	for _, addr := range coalescedAddrs {
		req := mem.NewAccessReq()
		req.SetSendTime(now)
		req.SetDst(u.cu.VectorMem)
		req.SetSrc(u.cu)
		req.Type = mem.Read
		req.ByteSize = 64 // Always read a cache line
		req.Address = addr
		req.Info = info

		u.cu.GetConnection("ToVectorMem").Send(req)
	}
}

func (u *VectorMemoryUnit) sendDataStoreRequest(
	coalescedAddrs []uint64,
	preCoalescedAddrs [64]uint64,
	data [256]uint32,
	now core.VTimeInSec,
) {
	info := new(MemAccessInfo)
	info.Action = MemAccessVectorDataLoad
	info.PreCoalescedAddrs = preCoalescedAddrs
	info.Wf = u.toExec
	info.Inst = info.Wf.inst
	info.Dst = info.Wf.inst.Dst.Register
	info.TotalReqs = len(coalescedAddrs)
	for _, addr := range coalescedAddrs {
		req := mem.NewAccessReq()
		req.SetSendTime(now)
		req.SetDst(u.cu.VectorMem)
		req.SetSrc(u.cu)
		req.Type = mem.Read
		req.ByteSize = 64 // Always read a cache line
		req.Address = addr
		req.Info = info
		req.Buf = make([]byte, 64)
		for i := 0; i < 64; i++ {
			currAddr := preCoalescedAddrs[i]
			addrCacheLineID := currAddr & 0xffffffffffffffc0
			addrCacheLineOffset := currAddr & 0x000000000000003f

			if addrCacheLineID != addr {
				continue
			} else {
				copy(req.Buf[addrCacheLineOffset:addrCacheLineOffset+4], insts.Uint32ToBytes(data[i*4]))
			}
		}

		u.cu.GetConnection("ToVectorMem").Send(req)
	}
}

func (u *VectorMemoryUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}
