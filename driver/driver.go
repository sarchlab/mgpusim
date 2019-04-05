package driver

import (
	"bytes"
	"encoding/binary"
	"log"
	"reflect"
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/vis/trace"
)

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.TickingComponent

	memAllocator memoryAllocator
	distributor  distributor

	GPUs []*gcn3.GPU
	MMU  vm.MMU

	requestsToSend []akita.Req

	contextMutex sync.Mutex
	contexts     []*Context

	ToGPUs akita.Port

	driverStopped chan bool
	enqueueSignal chan bool
	engineMutex   sync.Mutex
}

// Run starts a new threads that handles all commands in the command queues
func (d *Driver) Run() {
	go d.runAsync()
}

// Terminate stops the driver thread execution.
func (d *Driver) Terminate() {
	d.driverStopped <- true
}

func (d *Driver) runAsync() {
	for {
		select {
		case <-d.driverStopped:
			return
		case <-d.enqueueSignal:
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
	err := d.Engine.Run()
	if err != nil {
		panic(err)
	}
}

// RegisterGPU tells the driver about the existence of a GPU
func (d *Driver) RegisterGPU(gpu *gcn3.GPU, dramSize uint64) {
	d.GPUs = append(d.GPUs, gpu)
	d.memAllocator.RegisterStorage(dramSize)
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
	d.contextMutex.Lock()
	for _, context := range d.contexts {
		context.queueMutex.Lock()
		for _, q := range context.queues {
			if q.NumCommand() == 0 {
				continue
			}

			if q.IsRunning {
				continue
			}

			d.processOneCommand(now, q)
		}
		context.queueMutex.Unlock()
	}
	d.contextMutex.Unlock()
}

func (d *Driver) processOneCommand(
	now akita.VTimeInSec,
	cmdQueue *CommandQueue,
) {
	cmd := cmdQueue.Peek()

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

	d.logCmdStart(cmd, now)
}

func (d *Driver) logCmdStart(cmd Command, now akita.VTimeInSec) {
	task := trace.Task{
		ID:           cmd.GetID(),
		Type:         "Driver Command",
		InitiateTime: float64(now),
		What:         reflect.TypeOf(cmd).String(),
		Where:        d.Name(),
		Detail:       cmd,
	}
	ctx := akita.HookCtx{
		Domain: d,
		Now:    now,
		Pos:    trace.HookPosTaskInitiate,
		Item:   task,
	}
	d.InvokeHook(&ctx)
}

func (d *Driver) logCmdComplete(cmd Command, now akita.VTimeInSec) {
	task := trace.Task{
		ID: cmd.GetID(),
	}
	ctx := akita.HookCtx{
		Domain: d,
		Now:    now,
		Pos:    trace.HookPosTaskClear,
		Item:   task,
	}
	d.InvokeHook(&ctx)
}

func (d *Driver) processNoopCommand(
	now akita.VTimeInSec,
	cmd *NoopCommand,
	queue *CommandQueue,
) {
	queue.Dequeue()
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
		pAddr, page := d.MMU.Translate(queue.Context.pid, addr)
		if page == nil {
			panic("page not found")
		}
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := d.memAllocator.GetDeviceIDByPAddr(pAddr)
		req := gcn3.NewMemCopyH2DReq(now,
			d.ToGPUs, d.GPUs[gpuID-1].ToDriver,
			rawBytes[offset:offset+sizeToCopy],
			pAddr)
		cmd.Reqs = append(cmd.Reqs, req)
		d.requestsToSend = append(d.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		d.logTaskToGPUInitiate(now, cmd, req)
	}
	queue.IsRunning = true
	d.NeedTick = true
}

func (d *Driver) logTaskToGPUInitiate(
	now akita.VTimeInSec,
	cmd Command,
	req akita.Req,
) {
	task := trace.Task{
		ID:           req.GetID(),
		ParentID:     cmd.GetID(),
		Where:        d.Name(),
		Type:         "req",
		What:         reflect.TypeOf(req).String(),
		InitiateTime: float64(now),
	}
	ctx := akita.HookCtx{
		Domain: d,
		Now:    now,
		Pos:    trace.HookPosTaskInitiate,
		Item:   task,
	}
	d.InvokeHook(&ctx)
}

func (d *Driver) logTaskToGPUClear(
	now akita.VTimeInSec,
	req akita.Req,
) {
	task := trace.Task{
		ID: req.GetID(),
	}
	ctx := akita.HookCtx{
		Domain: d,
		Now:    now,
		Pos:    trace.HookPosTaskClear,
		Item:   task,
	}
	d.InvokeHook(&ctx)
}

func (d *Driver) processMemCopyH2DReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyH2DReq,
) {
	d.logTaskToGPUClear(now, req)

	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyH2DCommand)
	newReqs := make([]akita.Req, 0, len(copyCmd.Reqs)-1)
	for _, r := range copyCmd.GetReqs() {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	copyCmd.Reqs = newReqs

	if len(copyCmd.Reqs) == 0 {
		cmdQueue.IsRunning = false
		cmdQueue.Dequeue()

		d.logCmdComplete(cmd, now)
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
		pAddr, page := d.MMU.Translate(queue.Context.pid, addr)
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := d.memAllocator.GetDeviceIDByPAddr(pAddr)
		req := gcn3.NewMemCopyD2HReq(now,
			d.ToGPUs, d.GPUs[gpuID-1].ToDriver,
			pAddr, cmd.RawData[offset:offset+sizeToCopy])
		cmd.Reqs = append(cmd.Reqs, req)
		d.requestsToSend = append(d.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		d.logTaskToGPUInitiate(now, cmd, req)
	}

	queue.IsRunning = true
	d.NeedTick = true
}

func (d *Driver) processMemCopyD2HReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyD2HReq,
) {
	d.logTaskToGPUClear(now, req)

	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyD2HCommand)
	newReqs := make([]akita.Req, 0, len(copyCmd.Reqs)-1)
	for _, r := range copyCmd.GetReqs() {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	copyCmd.Reqs = newReqs

	if len(copyCmd.Reqs) == 0 {
		cmdQueue.IsRunning = false
		buf := bytes.NewReader(copyCmd.RawData)
		err := binary.Read(buf, binary.LittleEndian, copyCmd.Dst)
		if err != nil {
			panic(err)
		}

		cmdQueue.Dequeue()

		d.logCmdComplete(copyCmd, now)
	}

	d.NeedTick = true
}

func (d *Driver) processLaunchKernelCommand(
	now akita.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewLaunchKernelReq(now,
		d.ToGPUs, d.GPUs[queue.GPUID-1].ToDriver)
	req.PID = queue.Context.pid
	req.HsaCo = cmd.CodeObject

	req.Packet = cmd.Packet
	req.PacketAddress = uint64(cmd.DPacket)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)

	d.requestsToSend = append(d.requestsToSend, req)
	d.NeedTick = true

	d.logCmdStart(cmd, now)
	d.logTaskToGPUInitiate(now, cmd, req)
}

func (d *Driver) processLaunchKernelReturn(
	now akita.VTimeInSec,
	req *gcn3.LaunchKernelReq,
) {
	cmd, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Dequeue()
	d.NeedTick = true

	d.logTaskToGPUClear(now, req)
	d.logCmdComplete(cmd, now)
}

func (d *Driver) processFlushCommand(
	now akita.VTimeInSec,
	cmd *FlushCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewFlushCommand(now,
		d.ToGPUs, d.GPUs[queue.GPUID-1].ToDriver)

	d.requestsToSend = append(d.requestsToSend, req)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)
	d.NeedTick = true

	d.logCmdStart(cmd, now)
	d.logTaskToGPUInitiate(now, cmd, req)
}

func (d *Driver) processFlushReturn(
	now akita.VTimeInSec,
	req *gcn3.FlushCommand,
) {
	cmd, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Dequeue()
	d.NeedTick = true

	d.logTaskToGPUClear(now, req)
	d.logCmdComplete(cmd, now)
}

func (d *Driver) findCommandByReq(req akita.Req) (Command, *CommandQueue) {
	d.contextMutex.Lock()
	defer d.contextMutex.Unlock()

	for _, ctx := range d.contexts {
		ctx.queueMutex.Lock()
		for _, q := range ctx.queues {
			cmd := q.Peek()
			if cmd == nil {
				continue
			}

			reqs := cmd.GetReqs()
			for _, r := range reqs {
				if r == req {
					ctx.queueMutex.Unlock()
					return cmd, q
				}
			}
		}
		ctx.queueMutex.Unlock()
	}
	panic("cannot find command")
}

func (d *Driver) reserveMemoryForCPU() {
	d.memAllocator.RegisterStorage(1 << 32)
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine, mmu vm.MMU) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	memAllocatorImpl := newMemoryAllocatorImpl(mmu)
	memAllocatorImpl.pageSizeAsPowerOf2 = 12
	driver.memAllocator = memAllocatorImpl

	distributorImpl := newDistributorImpl(memAllocatorImpl)
	distributorImpl.pageSizeAsPowerOf2 = 12
	driver.distributor = distributorImpl

	driver.MMU = mmu

	driver.ToGPUs = akita.NewLimitNumReqPort(driver, 40960000)

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)

	driver.reserveMemoryForCPU()

	return driver
}
