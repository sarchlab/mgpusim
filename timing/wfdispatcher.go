package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
)

// WfDispatchingState represents to progress of a wavefront dispatching
type WfDispatchingState int

// A list of possible dispatching states
const (
	WfDispatchingNotStarted  WfDispatchingState = iota
	WfDispatchingInitialized                    // Inserted in the wavefront pool,
	WfDispatchingSRegSet                        // Done with sending s reg write request
	WfDispatchingVRegSet                        // Done with sending v reg write request
	WfDispatchingDone                           // All the register writing has completed
)

// DispatchWfEvent requires the scheduler shart to schedule for the event.
type DispatchWfEvent struct {
	*core.EventBase

	Req *gcn3.DispatchWfReq

	ManagedWf *Wavefront
	State     WfDispatchingState
}

// NewDispatchWfEvent returns a newly created DispatchWfEvent
func NewDispatchWfEvent(
	time core.VTimeInSec,
	handler core.Handler,
	req *gcn3.DispatchWfReq,
) *DispatchWfEvent {
	e := new(DispatchWfEvent)
	e.EventBase = core.NewEventBase(time, handler)
	e.Req = req
	return e
}

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(evt *DispatchWfEvent) (bool, *Wavefront)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	Scheduler *Scheduler
}

// DispatchWf starts or continues a wavefront dispatching process.
func (d *WfDispatcherImpl) DispatchWf(evt *DispatchWfEvent) (bool, *Wavefront) {
	req := evt.Req
	wf := req.Wf
	info := req.Info
	managedWf := evt.ManagedWf
	if managedWf != nil {
		managedWf.Lock()
		defer managedWf.Unlock()
	}

	for {
		switch evt.State {
		case WfDispatchingNotStarted:
			wfPool := d.Scheduler.WfPools[info.SIMDID]
			managedWf = new(Wavefront)

			managedWf.Lock()
			defer managedWf.Unlock()

			managedWf.Wavefront = wf
			managedWf.SIMDID = info.SIMDID
			managedWf.LDSOffset = info.LDSOffset
			managedWf.SRegOffset = info.SGPROffset
			managedWf.VRegOffset = info.VGPROffset
			managedWf.CodeObject = req.CodeObject
			managedWf.Packet = req.Packet
			wfPool.AddWf(managedWf)
			evt.ManagedWf = managedWf
			d.initCtrlRegs(evt)
			evt.State = WfDispatchingInitialized
		case WfDispatchingInitialized:
			done := d.initSRegs(managedWf, evt)
			if done {
				evt.State = WfDispatchingSRegSet
			} else {
				return false, nil
			}
		case WfDispatchingSRegSet:
			done := d.initVRegs(managedWf, evt)
			if done {
				evt.State = WfDispatchingVRegSet
			} else {
				return false, nil
			}
		case WfDispatchingVRegSet:
			done := d.allReqCompleted(evt)
			if done {
				evt.State = WfDispatchingDone
			} else {
				return false, nil
			}
		case WfDispatchingDone:
			managedWf.State = WfReady
			return true, managedWf
		}
	}
}

func (d *WfDispatcherImpl) initCtrlRegs(evt *DispatchWfEvent) {
	wf := evt.ManagedWf
	wf.PC = evt.Req.EntryPoint
}

func (d *WfDispatcherImpl) initSRegs(wf *Wavefront, evt *DispatchWfEvent) bool {
	req := evt.Req
	co := req.CodeObject
	packet := req.Packet
	now := evt.Time()
	count := 0

	if co.EnableSgprPrivateSegmentBuffer() {
		log.Println("Initializing register PrivateSegmentBuffer is not supported")
		count += 4
	}

	if co.EnableSgprDispatchPtr() {
		reg := insts.SReg(count)
		// FIXME: Fill in the correct value
		bytes := insts.Uint64ToBytes(0)
		d.Scheduler.writeReg(wf, reg, bytes, now)
		count += 2
	}

	if co.EnableSgprQueuePtr() {
		log.Println("Initializing register QueuePtr is not supported")
		count += 2
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		reg := insts.SReg(count)
		bytes := insts.Uint64ToBytes(packet.KernargAddress)
		d.Scheduler.writeReg(wf, reg, bytes, now)
		count += 2
	}

	if co.EnableSgprDispatchId() {
		log.Println("Initializing register DispatchId is not supported")
		count += 2
	}

	if co.EnableSgprFlatScratchInit() {
		log.Println("Initializing register FlatScratchInit is not supported")
		count += 2
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Println("Initializing register PrivateSegementSize is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountX() {
		log.Println("Initializing register GridWorkGroupCountX is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountY() {
		log.Println("Initializing register GridWorkGroupCountY is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		log.Println("Initializing register GridWorkGroupCountZ is not supported")
		count++
	}

	if co.EnableSgprWorkGroupIdX() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDX))
		d.Scheduler.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupIdY() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDY))
		d.Scheduler.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupIdZ() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDZ))
		d.Scheduler.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Println("Initializing register GridWorkGroupInfo is not supported")
		count++
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Println("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count++
	}

	return true
}

func (d *WfDispatcherImpl) initVRegs(wf *Wavefront, evt *DispatchWfEvent) bool {
	return true
}

func (d *WfDispatcherImpl) allReqCompleted(evt *DispatchWfEvent) bool {
	return true
}
