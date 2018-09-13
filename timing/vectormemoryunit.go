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

	ReadBuf      []*mem.ReadReq
	WriteBuf     []*mem.WriteReq
	ReadBufSize  int
	WriteBufSize int

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

	u.ReadBufSize = 256
	u.ReadBuf = make([]*mem.ReadReq, 0, u.ReadBufSize)

	u.WriteBufSize = 256
	u.WriteBuf = make([]*mem.WriteReq, 0, u.WriteBufSize)

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
func (u *VectorMemoryUnit) Run(now akita.VTimeInSec) {
	u.sendRequest(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *VectorMemoryUnit) runReadStage(now akita.VTimeInSec) {
	if u.toRead == nil {
		return
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)
		u.cu.InvokeHook(u.toRead, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toRead.inst, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, akita.AnyHookPos, &InstHookInfo{now, u.toRead.inst, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *VectorMemoryUnit) runExecStage(now akita.VTimeInSec) {
	if u.toExec == nil {
		return
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
	}
}

func (u *VectorMemoryUnit) executeFlatInsts(now akita.VTimeInSec) {
	u.toExec.OutstandingVectorMemAccess++
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
) []AddrSizePair {
	return u.coalescer.Coalesce(addresses, byteSizePerLane)
}

func (u *VectorMemoryUnit) bufferDataLoadRequest(
	coalescedAddrs []AddrSizePair,
	preCoalescedAddrs [64]uint64,
	registerCount int,
	now akita.VTimeInSec,
) {

	instLevelInfo := new(InstLevelInfo)
	instLevelInfo.Inst = u.toExec.inst
	instLevelInfo.TotalReqs = len(coalescedAddrs)
	instLevelInfo.ReturnedReqs = 0

	for _, addr := range coalescedAddrs {
		info := newMemAccessInfo()
		info.InstLevelInfo = instLevelInfo
		info.Action = MemAccessVectorDataLoad
		info.PreCoalescedAddrs = preCoalescedAddrs
		info.Wf = u.toExec
		info.Dst = info.Wf.inst.Dst.Register
		info.RegCount = registerCount
		info.Address = addr.Addr

		lowModule := u.cu.VectorMemModules.Find(addr.Addr)
		req := mem.NewReadReq(now, u.cu.ToVectorMem, lowModule, addr.Addr, addr.Size)
		u.cu.inFlightMemAccess[req.ID] = info
		u.ReadBuf = append(u.ReadBuf, req)
	}
}

func (u *VectorMemoryUnit) bufferDataStoreRequest(
	coalescedAddrs []AddrSizePair,
	preCoalescedAddrs [64]uint64,
	data [256]uint32,
	registerCount int,
	now akita.VTimeInSec,
) {
	instLevelInfo := new(InstLevelInfo)
	instLevelInfo.Inst = u.toExec.inst
	instLevelInfo.TotalReqs = len(preCoalescedAddrs)
	instLevelInfo.ReturnedReqs = 0

	for i, addr := range preCoalescedAddrs {
		info := newMemAccessInfo()
		info.InstLevelInfo = instLevelInfo
		info.Action = MemAccessVectorDataStore
		info.PreCoalescedAddrs = preCoalescedAddrs
		info.Wf = u.toExec
		info.Dst = info.Wf.inst.Dst.Register
		info.Address = addr

		lowModule := u.cu.VectorMemModules.Find(addr)
		req := mem.NewWriteReq(now, u.cu.ToVectorMem, lowModule, addr)
		req.Address = addr

		for j := 0; j < registerCount; j++ {
			req.Data = insts.Uint32ToBytes(data[i*4+j])
		}
		u.WriteBuf = append(u.WriteBuf, req)
		u.cu.inFlightMemAccess[req.ID] = info
	}
}

func (u *VectorMemoryUnit) sendRequest(now akita.VTimeInSec) {
	if len(u.ReadBuf) > 0 {
		req := u.ReadBuf[0]
		req.SetSendTime(now)
		err := u.cu.ToVectorMem.Send(req)
		if err == nil {
			u.ReadBuf = u.ReadBuf[1:]
		}
	}

	if len(u.WriteBuf) > 0 {
		req := u.WriteBuf[0]
		req.SetSendTime(now)
		err := u.cu.ToVectorMem.Send(req)
		if err == nil {
			u.WriteBuf = u.WriteBuf[1:]
		}
	}
}
