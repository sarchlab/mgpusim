package driver

import (
	"bytes"
	"encoding/binary"
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

// HookPosReqStart is a hook position that triggers hook when a request starts
var HookPosReqStart = &struct{ name string }{"Any"}

// HookPosReqReturn is a hook position that triggers hook when a request returns
// to the driver.
var HookPosReqReturn = &struct{ name string }{"Any"}

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.TickingComponent

	gpus        []*gcn3.GPU
	memoryMasks []*MemoryMask
	totalSize   uint64
	usingGPU    int

	CommandQueues []*CommandQueue

	ToGPUs akita.Port
}

// ExecuteAllCommands run the simulation until all the commands are completed.
func (d *Driver) ExecuteAllCommands() {
	now := d.Engine.CurrentTime()
	d.TickLater(now)

	err := d.Engine.Run()
	if err != nil {
		panic(err)
	}
}

// RegisterGPU tells the driver about the existence of a GPU
func (d *Driver) RegisterGPU(gpu *gcn3.GPU, dramSize uint64) {
	d.gpus = append(d.gpus, gpu)

	d.registerStorage(GPUPtr(d.totalSize), dramSize)
	d.totalSize += dramSize
}

// GetNumGPUs return the number of GPUs in the platform
func (d *Driver) GetNumGPUs() int {
	return len(d.gpus)
}

// SelectGPU requires the driver to perform the following APIs on a selected
// GPU
func (d *Driver) SelectGPU(gpuID int) {
	if gpuID >= len(d.gpus) {
		log.Panicf("no GPU %d in the system", gpuID)
	}
	d.usingGPU = gpuID
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

	d.processReturnReq(now)
	d.processNewCommand(now)

	if d.NeedTick {
		d.TickLater(now)
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

func (d *Driver) findCommandByReq(req akita.Req) (Command, *CommandQueue) {
	for _, cmdQueue := range d.CommandQueues {
		if len(cmdQueue.Commands) == 0 {
			continue
		}

		if cmdQueue.Commands[0].GetReq() == req {
			return cmdQueue.Commands[0], cmdQueue
		}
	}

	panic("cannot find command")
}

func (d *Driver) processNewCommand(now akita.VTimeInSec) {
	for _, cmdQueue := range d.CommandQueues {
		if len(cmdQueue.Commands) == 0 {
			continue
		}

		if cmdQueue.IsRunning {
			continue
		}

		d.processOneCommand(now, cmdQueue)
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
	default:
		log.Panicf("cannot process command of type %s", reflect.TypeOf(cmd))
	}
}

func (d *Driver) processMemCopyH2DCommand(
	now akita.VTimeInSec,
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) {
	rawData := make([]byte, 0)
	buffer := bytes.NewBuffer(rawData)

	err := binary.Write(buffer, binary.LittleEndian, cmd.Src)
	if err != nil {
		panic(err)
	}

	req := gcn3.NewMemCopyH2DReq(now,
		d.ToGPUs, d.gpus[queue.GPUID].ToDriver,
		buffer.Bytes(), uint64(cmd.Dst))
	sendError := d.ToGPUs.Send(req)
	if sendError == nil {
		queue.IsRunning = true
		cmd.Req = req
		d.NeedTick = true
	}
}

func (d *Driver) processMemCopyH2DReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyH2DReq,
) {
	_, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]

	d.NeedTick = true
}

func (d *Driver) processMemCopyD2HCommand(
	now akita.VTimeInSec,
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) {
	rawData := make([]byte, binary.Size(cmd.Dst))

	req := gcn3.NewMemCopyD2HReq(now,
		d.ToGPUs, d.gpus[queue.GPUID].ToDriver,
		uint64(cmd.Src), rawData)
	sendError := d.ToGPUs.Send(req)
	if sendError == nil {
		queue.IsRunning = true
		cmd.Req = req
		d.NeedTick = true
	}
}

func (d *Driver) processMemCopyD2HReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyD2HReq,
) {
	cmd, cmdQueue := d.findCommandByReq(req)

	memCopyCommand := cmd.(*MemCopyD2HCommand)

	buf := bytes.NewReader(req.DstBuffer)
	err := binary.Read(buf, binary.LittleEndian, memCopyCommand.Dst)
	if err != nil {
		panic(err)
	}

	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]
	d.NeedTick = true
}

func (d *Driver) processLaunchKernelCommand(
	now akita.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewLaunchKernelReq(now,
		d.ToGPUs, d.gpus[queue.GPUID].ToDriver)
	req.HsaCo = cmd.CodeObject
	req.Packet = cmd.Packet
	req.PacketAddress = uint64(cmd.DPacket)

	sendError := d.ToGPUs.Send(req)
	if sendError == nil {
		req.StartTime = now
		d.InvokeHook(req, d, HookPosReqStart, nil)

		queue.IsRunning = true
		cmd.Req = req
		d.NeedTick = true
	}
}

func (d *Driver) processLaunchKernelReturn(
	now akita.VTimeInSec,
	req *gcn3.LaunchKernelReq,
) {
	_, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]
	d.NeedTick = true

	req.EndTime = now
	d.InvokeHook(req, d, HookPosReqReturn, nil)
}

func (d *Driver) processFlushCommand(
	now akita.VTimeInSec,
	cmd *FlushCommand,
	queue *CommandQueue,
) {
	req := gcn3.NewFlushCommand(now,
		d.ToGPUs, d.gpus[queue.GPUID].ToDriver)

	sendError := d.ToGPUs.Send(req)
	if sendError == nil {
		req.StartTime = now
		d.InvokeHook(req, d, HookPosReqStart, nil)

		queue.IsRunning = true
		cmd.Req = req
		d.NeedTick = true
	}
}

func (d *Driver) processFlushReturn(
	now akita.VTimeInSec,
	req *gcn3.FlushCommand,
) {
	_, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Commands = cmdQueue.Commands[1:]
	d.NeedTick = true

	req.EndTime = now
	d.InvokeHook(req, d, HookPosReqReturn, nil)
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	driver.memoryMasks = make([]*MemoryMask, 0)

	driver.ToGPUs = akita.NewLimitNumReqPort(driver, 1)

	return driver
}
