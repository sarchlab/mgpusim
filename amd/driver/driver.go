package driver

import (
	"log"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/tebeka/atexit"
)

// Driver is an Akita component that controls the simulated GPUs. It exposes
// the host-facing API used by the benchmarks and forwards the resulting
// commands to the GPUs' command processors through its "GPU" port.
type Driver struct {
	*Comp

	// TODO(akita5): state purity. The fields below are complex runtime state
	// (pointers, interfaces, channels, mutexes) that cannot live in the
	// checkpointable State struct yet.
	memAllocator internal.MemoryAllocator
	distributor  distributor

	GPUs        []messaging.RemotePort
	devices     []*internal.Device
	middlewares []Middleware

	requestsToSend []messaging.Msg

	contextMutex sync.Mutex
	contexts     []*Context

	engine             timing.Engine
	driverStopped      chan bool
	enqueueSignal      chan bool
	engineMutex        sync.Mutex
	engineRunning      bool
	rerunNeeded        bool
	engineRunningMutex sync.Mutex
	simulationID       uint64

	Log2PageSize uint64

	codeObjGPUAddrs map[*insts.KernelCodeObject]Ptr
}

// gpuPort returns the port that connects the driver to the GPUs' command
// processors. The port instance is assigned externally after Build.
func (d *Driver) gpuPort() messaging.Port {
	return d.GetPortByName(GPUPortName)
}

// Run starts a new thread that handles all commands in the command queues.
func (d *Driver) Run() {
	d.logSimulationStart()
	go d.runAsync()
}

// Terminate stops the driver thread execution.
func (d *Driver) Terminate() {
	d.driverStopped <- true
	d.logSimulationTerminate()
}

func (d *Driver) logSimulationStart() {
	d.simulationID = timing.GetIDGenerator().Generate()
	tracing.StartTask(d, tracing.TaskStart{
		ID:   d.simulationID,
		Kind: "Simulation",
		What: "Simulation",
	})
}

func (d *Driver) logSimulationTerminate() {
	tracing.EndTask(d, tracing.TaskEnd{ID: d.simulationID})
}

func (d *Driver) runAsync() {
	for {
		select {
		case <-d.driverStopped:
			return
		case <-d.enqueueSignal:
			// Schedule the driver tick and decide whether to (re)start the
			// engine goroutine. The whole decision is made under
			// engineRunningMutex so it is atomic with respect to runEngine
			// clearing engineRunning when the event queue drains. Without this,
			// a wakeup that arrives in the window between engine.Run() returning
			// and engineRunning being cleared would be lost: runAsync would see
			// engineRunning==true and trust an engine that is already exiting,
			// leaving the freshly scheduled tick stranded with no runner.
			d.engineRunningMutex.Lock()

			d.engine.Pause()
			d.TickLater()
			d.engine.Continue()

			if d.engineRunning {
				// An engine goroutine owns the queue. Record that more work
				// arrived so it re-runs after draining instead of exiting.
				d.rerunNeeded = true
				d.engineRunningMutex.Unlock()
				continue
			}

			d.engineRunning = true
			d.rerunNeeded = false
			go d.runEngine()
			d.engineRunningMutex.Unlock()
		}
	}
}

func (d *Driver) runEngine() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic: %v", r)
			debug.PrintStack()
			atexit.Exit(1)
		}
	}()

	for {
		d.engineMutex.Lock()
		err := d.engine.Run()
		d.engineMutex.Unlock()
		if err != nil {
			panic(err)
		}

		// The queue drained. Re-check, under the same lock runAsync uses, for
		// a wakeup that raced in while we were running. If one did, loop and
		// run again; otherwise mark the engine stopped. This check and the
		// engineRunning check in runAsync are mutually exclusive, so no tick is
		// ever left behind without a running engine.
		d.engineRunningMutex.Lock()
		if d.rerunNeeded {
			d.rerunNeeded = false
			d.engineRunningMutex.Unlock()
			continue
		}
		d.engineRunning = false
		d.engineRunningMutex.Unlock()

		return
	}
}

// DeviceProperties defines the properties of a device
type DeviceProperties struct {
	CUCount  int
	DRAMSize uint64
}

// RegisterGPU tells the driver about the existence of a GPU. The port is the
// command processor's driver-facing port.
func (d *Driver) RegisterGPU(
	commandProcessorPort messaging.RemotePort,
	properties DeviceProperties,
) {
	d.GPUs = append(d.GPUs, commandProcessorPort)

	gpuDevice := &internal.Device{
		ID:       len(d.GPUs),
		Type:     internal.DeviceTypeGPU,
		MemState: internal.NewDeviceMemoryState(d.Log2PageSize),
		Properties: internal.DeviceProperties{
			CUCount:  properties.CUCount,
			DRAMSize: properties.DRAMSize,
		},
	}
	gpuDevice.SetTotalMemSize(properties.DRAMSize)
	d.memAllocator.RegisterDevice(gpuDevice)

	d.devices = append(d.devices, gpuDevice)
}

// sendMW sends the pending driver-to-GPU requests. It is the first
// middleware in the tick order, mirroring the v4 tick function.
type sendMW struct {
	driver *Driver
}

func (m *sendMW) Tick() bool {
	return m.driver.sendToGPUs()
}

// driverMiddlewaresMW ticks the driver-level middlewares (memory copy
// handling). It runs after sending and before return/command processing,
// mirroring the v4 tick order.
type driverMiddlewaresMW struct {
	driver *Driver
}

func (m *driverMiddlewaresMW) Tick() bool {
	madeProgress := false

	for _, mw := range m.driver.middlewares {
		madeProgress = mw.Tick() || madeProgress
	}

	return madeProgress
}

// commandMW processes the responses returned from the GPUs and the new
// commands in the command queues. It is the last middleware in the tick
// order, mirroring the v4 tick function.
type commandMW struct {
	driver *Driver
}

func (m *commandMW) Tick() bool {
	madeProgress := m.driver.processReturnReq()
	madeProgress = m.driver.processNewCommand() || madeProgress

	return madeProgress
}

func (d *Driver) sendToGPUs() bool {
	if len(d.requestsToSend) == 0 {
		return false
	}

	port := d.gpuPort()
	if !port.CanSend() {
		return false
	}

	req := d.requestsToSend[0]
	port.Send(req)
	d.requestsToSend = d.requestsToSend[1:]

	return true
}

func (d *Driver) processReturnReq() bool {
	req := d.gpuPort().PeekIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case protocol.LaunchKernelRsp:
		d.gpuPort().RetrieveIncoming()
		return d.processLaunchKernelReturn(req)
	}

	return false
}

func (d *Driver) processNewCommand() bool {
	madeProgress := false

	d.contextMutex.Lock()
	for _, ctx := range d.contexts {
		madeProgress = d.processNewCommandFromContext(ctx) || madeProgress
	}
	d.contextMutex.Unlock()

	return madeProgress
}

func (d *Driver) processNewCommandFromContext(
	ctx *Context,
) bool {
	madeProgress := false
	ctx.queueMutex.Lock()
	for _, q := range ctx.queues {
		madeProgress = d.processNewCommandFromCmdQueue(q) || madeProgress
	}
	ctx.queueMutex.Unlock()

	return madeProgress
}

func (d *Driver) processNewCommandFromCmdQueue(
	q *CommandQueue,
) bool {
	if q.NumCommand() == 0 {
		return false
	}

	if q.IsRunning {
		return false
	}

	return d.processOneCommand(q)
}

func (d *Driver) processOneCommand(
	cmdQueue *CommandQueue,
) bool {
	cmd := cmdQueue.Peek()

	switch cmd := cmd.(type) {
	case *LaunchKernelCommand:
		d.logCmdStart(cmd)
		return d.processLaunchKernelCommand(cmd, cmdQueue)
	case *NoopCommand:
		d.logCmdStart(cmd)
		return d.processNoopCommand(cmd, cmdQueue)
	case *LaunchUnifiedMultiGPUKernelCommand:
		d.logCmdStart(cmd)
		return d.processUnifiedMultiGPULaunchKernelCommand(cmd, cmdQueue)
	default:
		return d.processCommandWithMiddleware(cmd, cmdQueue)
	}
}

func (d *Driver) processCommandWithMiddleware(
	cmd Command,
	cmdQueue *CommandQueue,
) bool {
	for _, m := range d.middlewares {
		processed := m.ProcessCommand(cmd, cmdQueue)

		if processed {
			d.logCmdStart(cmd)
			return true
		}
	}

	return false
}

func (d *Driver) logCmdStart(cmd Command) {
	tracing.StartTask(d, tracing.TaskStart{
		ID:       cmd.GetID(),
		ParentID: d.simulationID,
		Kind:     "Driver Command",
		What:     reflect.TypeOf(cmd).String(),
	})
}

func (d *Driver) logCmdComplete(cmd Command) {
	tracing.EndTask(d, tracing.TaskEnd{ID: cmd.GetID()})
}

func (d *Driver) processNoopCommand(
	cmd *NoopCommand,
	queue *CommandQueue,
) bool {
	queue.Dequeue()
	return true
}

func (d *Driver) logTaskToGPUInitiate(
	cmd Command,
	req messaging.Msg,
) {
	tracing.TraceReqInitiate(d, req, cmd.GetID())
}

func (d *Driver) logTaskToGPUClear(
	req messaging.Msg,
) {
	tracing.TraceReqFinalize(d, req)
}

func (d *Driver) processLaunchKernelCommand(
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) bool {
	req := protocol.LaunchKernelReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: d.gpuPort().AsRemote(),
			Dst: d.GPUs[queue.GPUID-1],
		},
		PID:           queue.Context.pid,
		CodeObject:    cmd.CodeObject,
		Packet:        cmd.Packet,
		PacketAddress: uint64(cmd.DPacket),
	}

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)

	d.requestsToSend = append(d.requestsToSend, req)

	queue.Context.l2Dirty = true
	queue.Context.markAllBuffersDirty()

	d.logTaskToGPUInitiate(cmd, req)

	return true
}

func (d *Driver) processUnifiedMultiGPULaunchKernelCommand(
	cmd *LaunchUnifiedMultiGPUKernelCommand,
	queue *CommandQueue,
) bool {
	wgDist := d.distributeWGToGPUs(queue, cmd)

	dev := d.devices[queue.GPUID]
	for i, gpuID := range dev.UnifiedGPUIDs {
		if wgDist[i+1]-wgDist[i] == 0 {
			continue
		}

		currentGPUIndex := i
		req := protocol.LaunchKernelReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: d.gpuPort().AsRemote(),
				Dst: d.GPUs[gpuID-1],
			},
			PID:           queue.Context.pid,
			CodeObject:    cmd.CodeObject,
			Packet:        cmd.PacketArray[i],
			PacketAddress: uint64(cmd.DPacketArray[i]),
			WGFilter: func(
				pkt *kernels.HsaKernelDispatchPacket,
				wg *kernels.WorkGroup,
			) bool {
				numWGX := (pkt.GridSizeX-1)/uint32(pkt.WorkgroupSizeX) + 1
				numWGY := (pkt.GridSizeY-1)/uint32(pkt.WorkgroupSizeY) + 1

				flattenedID :=
					wg.IDZ*int(numWGX)*int(numWGY) +
						wg.IDY*int(numWGX) +
						wg.IDX

				if flattenedID >= wgDist[currentGPUIndex] &&
					flattenedID < wgDist[currentGPUIndex+1] {
					return true
				}

				return false
			},
		}

		queue.IsRunning = true
		cmd.Reqs = append(cmd.Reqs, req)

		d.requestsToSend = append(d.requestsToSend, req)

		queue.Context.l2Dirty = true
		queue.Context.markAllBuffersDirty()

		d.logTaskToGPUInitiate(cmd, req)
	}

	return true
}

func (d *Driver) distributeWGToGPUs(
	queue *CommandQueue,
	cmd *LaunchUnifiedMultiGPUKernelCommand,
) []int {
	dev := d.devices[queue.GPUID]
	actualGPUs := dev.UnifiedGPUIDs
	wgAllocated := 0
	wgDist := make([]int, len(actualGPUs)+1)

	totalCUCount := 0
	for _, devID := range actualGPUs {
		totalCUCount += d.devices[devID].Properties.CUCount
	}

	numWGX := (cmd.PacketArray[0].GridSizeX-1)/uint32(cmd.PacketArray[0].WorkgroupSizeX) + 1
	numWGY := (cmd.PacketArray[0].GridSizeY-1)/uint32(cmd.PacketArray[0].WorkgroupSizeY) + 1
	numWGZ := (cmd.PacketArray[0].GridSizeZ-1)/uint32(cmd.PacketArray[0].WorkgroupSizeZ) + 1
	totalWGCount := int(numWGX * numWGY * numWGZ)
	wgPerCU := (totalWGCount-1)/totalCUCount + 1

	for i, devID := range actualGPUs {
		cuCount := d.devices[devID].Properties.CUCount
		wgToAllocate := cuCount * wgPerCU
		wgDist[i+1] = wgAllocated + wgToAllocate
		wgAllocated += wgToAllocate
	}

	if wgAllocated < totalWGCount {
		panic("not all wg allocated")
	}

	return wgDist
}

func (d *Driver) processLaunchKernelReturn(
	rsp protocol.LaunchKernelRsp,
) bool {
	req, cmd, cmdQueue := d.findCommandByReqID(rsp.RspTo)
	cmd.RemoveReq(req)

	d.logTaskToGPUClear(req)

	if len(cmd.GetReqs()) == 0 {
		cmdQueue.IsRunning = false
		cmdQueue.Dequeue()

		d.logCmdComplete(cmd)
	}

	return true
}

func (d *Driver) findCommandByReqID(reqID uint64) (
	messaging.Msg,
	Command,
	*CommandQueue,
) {
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
				if r.Meta().ID == reqID {
					ctx.queueMutex.Unlock()
					return r, cmd, q
				}
			}
		}

		ctx.queueMutex.Unlock()
	}

	panic("cannot find command")
}
