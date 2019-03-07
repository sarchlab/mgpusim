package driver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/mem/vm"
)

// HookPosCommandStart is a hook position that triggers hook when a request
// starts
var HookPosCommandStart = &struct{ name string }{"CommandStart"}

// HookPosCommandComplete is a hook position that triggers hook when a request
// returns to the driver.
var HookPosCommandComplete = &struct{ name string }{"CommandComplete"}

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.TickingComponent

	GPUs                 []*gcn3.GPU
	MMU                  vm.MMU
	allocatedPages       [][]*vm.Page
	initialAddresses     []uint64
	storageSizes         []uint64
	memoryMasks          [][]*MemoryChunk
	totalStorageByteSize uint64

	PageSizeAsPowerOf2 uint64
	requestsToSend     []akita.Req

	Contexts []*Context

	ToGPUs akita.Port

	driverStopped chan bool
	enqueueSignal chan bool
	engineMutex   sync.Mutex

	memoryAllocatorLock sync.Mutex
}

func (d *Driver) Run() {
	go d.runAsync()
}

func (d *Driver) Terminate() {
	d.driverStopped <- true
}

func (d *Driver) runAsync() {
	fmt.Printf("Running\n")
	defer fmt.Printf("Driver thread exited\n")
	for {
		select {
		case <-d.driverStopped:
			return
		case <-d.enqueueSignal:
			fmt.Printf("enqueue signal received\n")
			d.Engine.Pause()
			d.TickLater(d.Engine.CurrentTime())
			d.Engine.Continue()
			go d.runEngine()
		}
	}
}

func (d *Driver) runEngine() {
	d.engineMutex.Lock()
	defer d.engineMutex.Unlock()
	fmt.Printf("Engine start\n")
	err := d.Engine.Run()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Engine return\n")
}

// RegisterGPU tells the driver about the existence of a GPU
func (d *Driver) RegisterGPU(gpu *gcn3.GPU, dramSize uint64) {
	d.GPUs = append(d.GPUs, gpu)

	d.registerStorage(GPUPtr(d.totalStorageByteSize), dramSize)
	d.totalStorageByteSize += dramSize
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e akita.Event) error {
	switch e := e.(type) {
	case akita.TickEvent:
		d.handleTickEvent(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (d *Driver) handleTickEvent(evt akita.TickEvent) {
	now := evt.Time()
	d.NeedTick = false

	d.sendToGPUs(now)
	d.processReturnReq(now)
	d.processNewCommand(now)

	if d.NeedTick {
		d.TickLater(now)
	}
}

func (d *Driver) sendToGPUs(now akita.VTimeInSec) {
	if len(d.requestsToSend) == 0 {
		return
	}

	req := d.requestsToSend[0]
	req.SetSendTime(now)
	err := d.ToGPUs.Send(req)
	if err == nil {
		d.requestsToSend = d.requestsToSend[1:]
		d.NeedTick = true
	}
}

func (d *Driver) processReturnReq(now akita.VTimeInSec) {
	req := d.ToGPUs.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *gcn3.MemCopyH2DReq:
		d.processMemCopyH2DReturn(now, req)
	case *gcn3.MemCopyD2HReq:
		d.processMemCopyD2HReturn(now, req)
	case *gcn3.LaunchKernelReq:
		d.processLaunchKernelReturn(now, req)
	case *gcn3.FlushCommand:
		d.processFlushReturn(now, req)
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}
}

func (d *Driver) processNewCommand(now akita.VTimeInSec) {
	for _, context := range d.Contexts {
		for _, q := range context.Queues {
			if len(q.Commands) == 0 {
				continue
			}

			if q.IsRunning {
				continue
			}

			d.processOneCommand(now, q)
		}
	}
}

func (d *Driver) processOneCommand(
	now akita.VTimeInSec,
	cmdQueue *CommandQueue,
) {
	cmd := cmdQueue.Commands[0]

	switch cmd := cmd.(type) {
	case *MemCopyH2DCommand:
		d.processMemCopyH2DCommand(now, cmd, cmdQueue)
	case *MemCopyD2HCommand:
		d.processMemCopyD2HCommand(now, cmd, cmdQueue)
	case *LaunchKernelCommand:
		d.processLaunchKernelCommand(now, cmd, cmdQueue)
	case *FlushCommand:
		d.processFlushCommand(now, cmd, cmdQueue)
	case *NoopCommand:
		d.processNoopCommand(now, cmd, cmdQueue)
	default:
		log.Panicf("cannot process command of type %s", reflect.TypeOf(cmd))
	}
}

func (d *Driver) processNoopCommand(
	now akita.VTimeInSec,
	cmd *NoopCommand,
	queue *CommandQueue,
) {
	queue.Commands = queue.Commands[1:]
	queue.NotifyAllSubscribers()
	d.TickLater(now)
}

func (d *Driver) processMemCopyH2DCommand(
	now akita.VTimeInSec,
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) {
	buffer := bytes.NewBuffer(nil)
	err := binary.Write(buffer, binary.LittleEndian, cmd.Src)
	if err != nil {
		panic(err)
	}
	rawBytes := buffer.Bytes()

	offset := uint64(0)
	addr := uint64(cmd.Dst)
	sizeLeft := uint64(len(rawBytes))
	for sizeLeft > 0 {
		pAddr, page := d.MMU.Translate(queue.Context.PID, addr)
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := d.findGPUIDByPAddr(pAddr)
		req := gcn3.NewMemCopyH2DReq(now,
			d.ToGPUs, d.GPUs[gpuID].ToDriver,
			rawBytes[offset:offset+sizeToCopy],
			pAddr)
		cmd.Reqs = append(cmd.Reqs, req)
		d.requestsToSend = append(d.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		d.InvokeHook(req, d, akita.AnyHookPos,
			&ReqHookInfo{
				Now:       now,
				CommandID: cmd.ID,
				EventType: "CREATE",
			})
	}
	queue.IsRunning = true
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandStart,
		&CommandHookInfo{
			Now:     now,
			IsStart: true,
			Queue:   queue,
		},
	)
}

func (d *Driver) processMemCopyH2DReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyH2DReq,
) {
	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyH2DCommand)
	newReqs := make([]akita.Req, 0, len(copyCmd.Reqs)-1)
	for _, r := range copyCmd.GetReqs() {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	copyCmd.Reqs = newReqs

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: copyCmd.ID,
			EventType: "RETRIEVE",
		})

	if len(copyCmd.Reqs) == 0 {
		cmdQueue.IsRunning = false
		cmdQueue.Commands = cmdQueue.Commands[1:]

		d.InvokeHook(copyCmd, d, HookPosCommandComplete,
			&CommandHookInfo{
				Now:     now,
				IsStart: false,
				Queue:   cmdQueue,
			})

		cmdQueue.NotifyAllSubscribers()
	}

	d.NeedTick = true
}

func (d *Driver) processMemCopyD2HCommand(
	now akita.VTimeInSec,
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) {
	cmd.RawData = make([]byte, binary.Size(cmd.Dst))

	offset := uint64(0)
	addr := uint64(cmd.Src)
	sizeLeft := uint64(len(cmd.RawData))
	for sizeLeft > 0 {
		pAddr, page := d.MMU.Translate(queue.Context.PID, addr)
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := d.findGPUIDByPAddr(pAddr)
		req := gcn3.NewMemCopyD2HReq(now,
			d.ToGPUs, d.GPUs[gpuID].ToDriver,
			pAddr, cmd.RawData[offset:offset+sizeToCopy])
		cmd.Reqs = append(cmd.Reqs, req)
		d.requestsToSend = append(d.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		d.InvokeHook(req, d, akita.AnyHookPos,
			&ReqHookInfo{
				Now:       now,
				CommandID: cmd.ID,
				EventType: "CREATE",
			})
	}

	queue.IsRunning = true
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandStart,
		&CommandHookInfo{
			Now:     now,
			IsStart: true,
			Queue:   queue,
		})
}

func (d *Driver) processMemCopyD2HReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyD2HReq,
) {
	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyD2HCommand)
	newReqs := make([]akita.Req, 0, len(copyCmd.Reqs)-1)
	for _, r := range copyCmd.GetReqs() {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	copyCmd.Reqs = newReqs

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: copyCmd.ID,
			EventType: "RETRIEVE",
		})

	if len(copyCmd.Reqs) == 0 {

		cmdQueue.IsRunning = false
		cmdQueue.Commands = cmdQueue.Commands[1:]
		buf := bytes.NewReader(copyCmd.RawData)
		err := binary.Read(buf, binary.LittleEndian, copyCmd.Dst)
		if err != nil {
			panic(err)
		}

		d.InvokeHook(copyCmd, d, HookPosCommandComplete,
			&CommandHookInfo{
				Now:     now,
				IsStart: false,
				Queue:   cmdQueue,
			})

		cmdQueue.NotifyAllSubscribers()
	}

	d.NeedTick = true
}

func (d *Driver) processLaunchKernelCommand(
	now akita.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewLaunchKernelReq(now,
		d.ToGPUs, d.GPUs[queue.GPUID].ToDriver)
	req.PID = queue.Context.PID
	req.HsaCo = cmd.CodeObject

	req.Packet = cmd.Packet
	req.PacketAddress = uint64(cmd.DPacket)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)

	d.requestsToSend = append(d.requestsToSend, req)
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandStart,
		&CommandHookInfo{
			Now:     now,
			IsStart: true,
			Queue:   queue,
		})

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: cmd.ID,
			EventType: "CREATE",
		})
}

func (d *Driver) processLaunchKernelReturn(
	now akita.VTimeInSec,
	req *gcn3.LaunchKernelReq,
) {
	// fmt.Printf("%.12f kernel return, start at %.12f\n", now, req.StartTime)

	_, cmdQueue := d.findCommandByReq(req)
	cmd := cmdQueue.Commands[0]
	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandComplete,
		&CommandHookInfo{
			Now:     now,
			IsStart: false,
			Queue:   cmdQueue,
		})

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: cmd.GetID(),
			EventType: "RETRIEVE",
		})
}

func (d *Driver) processFlushCommand(
	now akita.VTimeInSec,
	cmd *FlushCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewFlushCommand(now,
		d.ToGPUs, d.GPUs[queue.GPUID].ToDriver)

	d.requestsToSend = append(d.requestsToSend, req)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandStart,
		&CommandHookInfo{
			Now:     now,
			IsStart: true,
			Queue:   queue,
		})

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: cmd.GetID(),
			EventType: "CREATE",
		})
}

func (d *Driver) processFlushReturn(
	now akita.VTimeInSec,
	req *gcn3.FlushCommand,
) {
	_, cmdQueue := d.findCommandByReq(req)
	cmd := cmdQueue.Commands[0]
	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]
	cmdQueue.NotifyAllSubscribers()
	d.NeedTick = true

	d.InvokeHook(cmd, d, HookPosCommandComplete,
		&CommandHookInfo{
			Now:     now,
			IsStart: false,
			Queue:   cmdQueue,
		})

	d.InvokeHook(req, d, akita.AnyHookPos,
		&ReqHookInfo{
			Now:       now,
			CommandID: cmd.GetID(),
			EventType: "RETRIEVE",
		})
}

func (d *Driver) findCommandByReq(req akita.Req) (Command, *CommandQueue) {
	for _, ctx := range d.Contexts {
		for _, q := range ctx.Queues {
			if len(q.Commands) == 0 {
				continue
			}

			reqs := q.Commands[0].GetReqs()
			for _, r := range reqs {
				if r == req {
					return q.Commands[0], q
				}
			}

		}
	}
	panic("cannot find command")
}

func (d *Driver) findGPUIDByPAddr(pAddr uint64) int {
	for i := range d.GPUs {
		if pAddr >= d.initialAddresses[i] &&
			pAddr < d.initialAddresses[i]+d.storageSizes[i] {
			return i
		}
	}
	panic("never")
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine, mmu vm.MMU) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	driver.MMU = mmu
	driver.PageSizeAsPowerOf2 = 12

	driver.ToGPUs = akita.NewLimitNumReqPort(driver, 40960000)

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)

	return driver
}
