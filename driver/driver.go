package driver

import (
	"log"
	"reflect"
	"sync"

	"github.com/rs/xid"
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/mem/v2/vm"
	"gitlab.com/akita/mgpusim/v2/driver/internal"
	"gitlab.com/akita/mgpusim/v2/kernels"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/util/v2/ca"
	"gitlab.com/akita/util/v2/tracing"
)

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*sim.TickingComponent

	memAllocator  internal.MemoryAllocator
	distributor   distributor
	globalStorage *mem.Storage

	GPUs        []sim.Port
	devices     []*internal.Device
	pageTable   vm.PageTable
	middlewares []Middleware

	requestsToSend []sim.Msg

	contextMutex sync.Mutex
	contexts     []*Context

	mmuPort sim.Port
	gpuPort sim.Port

	driverStopped      chan bool
	enqueueSignal      chan bool
	engineMutex        sync.Mutex
	engineRunning      bool
	engineRunningMutex sync.Mutex
	simulationID       string

	Log2PageSize uint64

	currentPageMigrationReq         *vm.PageMigrationReqToDriver
	toSendToMMU                     *vm.PageMigrationRspFromDriver
	migrationReqToSendToCP          []*protocol.PageMigrationReqToCP
	isCurrentlyHandlingMigrationReq bool
	numRDMADrainACK                 uint64
	numRDMARestartACK               uint64
	numShootDownACK                 uint64
	numRestartACK                   uint64
	numPagesMigratingACK            uint64
	isCurrentlyMigratingOnePage     bool

	RemotePMCPorts []sim.Port
}

// Run starts a new threads that handles all commands in the command queues
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
	d.simulationID = xid.New().String()
	tracing.StartTask(
		d.simulationID,
		"",
		0,
		d,
		"Simulation", "Simulation",
		nil,
	)
}

func (d *Driver) logSimulationTerminate() {
	tracing.EndTask(d.simulationID, d.Engine.CurrentTime(), d)
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

			d.engineRunningMutex.Lock()
			if d.engineRunning {
				d.engineRunningMutex.Unlock()
				continue
			}

			d.engineRunning = true
			go d.runEngine()
			d.engineRunningMutex.Unlock()
		}
	}
}

func (d *Driver) runEngine() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic: %v", r)
		}
	}()

	d.engineMutex.Lock()
	defer d.engineMutex.Unlock()
	err := d.Engine.Run()
	if err != nil {
		panic(err)
	}

	d.engineRunningMutex.Lock()
	d.engineRunning = false
	d.engineRunningMutex.Unlock()
}

// RegisterGPU tells the driver about the existence of a GPU
func (d *Driver) RegisterGPU(commandProcessorPort sim.Port, dramSize uint64) {
	d.GPUs = append(d.GPUs, commandProcessorPort)

	gpuDevice := &internal.Device{
		ID:       len(d.GPUs),
		Type:     internal.DeviceTypeGPU,
		MemState: internal.NewDeviceMemoryState(d.Log2PageSize),
	}
	gpuDevice.SetTotalMemSize(dramSize)
	d.memAllocator.RegisterDevice(gpuDevice)

	d.devices = append(d.devices, gpuDevice)
}

// Tick ticks
func (d *Driver) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = d.sendToGPUs(now) || madeProgress
	madeProgress = d.sendToMMU(now) || madeProgress
	madeProgress = d.sendMigrationReqToCP(now) || madeProgress

	for _, mw := range d.middlewares {
		madeProgress = mw.Tick(now) || madeProgress
	}

	madeProgress = d.processReturnReq(now) || madeProgress
	madeProgress = d.processNewCommand(now) || madeProgress
	madeProgress = d.parseFromMMU(now) || madeProgress

	return madeProgress
}

func (d *Driver) sendToGPUs(now sim.VTimeInSec) bool {
	if len(d.requestsToSend) == 0 {
		return false
	}

	req := d.requestsToSend[0]
	req.Meta().SendTime = now
	err := d.gpuPort.Send(req)
	if err == nil {
		d.requestsToSend = d.requestsToSend[1:]
		return true
	}

	return false
}

//nolint:gocyclo
func (d *Driver) processReturnReq(now sim.VTimeInSec) bool {
	req := d.gpuPort.Peek()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *protocol.LaunchKernelReq:
		d.gpuPort.Retrieve(now)
		return d.processLaunchKernelReturn(now, req)
	case *protocol.RDMADrainRspToDriver:
		d.gpuPort.Retrieve(now)
		return d.processRDMADrainRsp(now, req)
	case *protocol.ShootDownCompleteRsp:
		d.gpuPort.Retrieve(now)
		return d.processShootdownCompleteRsp(now, req)
	case *protocol.PageMigrationRspToDriver:
		d.gpuPort.Retrieve(now)
		return d.processPageMigrationRspFromCP(now, req)
	case *protocol.RDMARestartRspToDriver:
		d.gpuPort.Retrieve(now)
		return d.processRDMARestartRspToDriver(now, req)
	case *protocol.GPURestartRsp:
		d.gpuPort.Retrieve(now)
		return d.handleGPURestartRsp(now, req)
	}

	return false
}

func (d *Driver) processNewCommand(now sim.VTimeInSec) bool {
	madeProgress := false

	d.contextMutex.Lock()
	for _, ctx := range d.contexts {
		madeProgress = d.processNewCommandFromContext(now, ctx) || madeProgress
	}
	d.contextMutex.Unlock()

	return madeProgress
}

func (d *Driver) processNewCommandFromContext(
	now sim.VTimeInSec,
	ctx *Context,
) bool {
	madeProgress := false
	ctx.queueMutex.Lock()
	for _, q := range ctx.queues {
		madeProgress = d.processNewCommandFromCmdQueue(now, q) || madeProgress
	}
	ctx.queueMutex.Unlock()

	return madeProgress
}

func (d *Driver) processNewCommandFromCmdQueue(
	now sim.VTimeInSec,
	q *CommandQueue,
) bool {
	if q.NumCommand() == 0 {
		return false
	}

	if q.IsRunning {
		return false
	}

	return d.processOneCommand(now, q)
}

func (d *Driver) processOneCommand(
	now sim.VTimeInSec,
	cmdQueue *CommandQueue,
) bool {
	cmd := cmdQueue.Peek()

	switch cmd := cmd.(type) {
	case *LaunchKernelCommand:
		d.logCmdStart(cmd, now)
		return d.processLaunchKernelCommand(now, cmd, cmdQueue)
	case *NoopCommand:
		d.logCmdStart(cmd, now)
		return d.processNoopCommand(now, cmd, cmdQueue)
	default:
		return d.processCommandWithMiddleware(now, cmd, cmdQueue)
	}
}

func (d *Driver) processCommandWithMiddleware(
	now sim.VTimeInSec,
	cmd Command,
	cmdQueue *CommandQueue,
) bool {
	for _, m := range d.middlewares {
		processed := m.ProcessCommand(now, cmd, cmdQueue)

		if processed {
			d.logCmdStart(cmd, now)
			return true
		}
	}

	return false
}

func (d *Driver) logCmdStart(cmd Command, now sim.VTimeInSec) {
	tracing.StartTask(
		cmd.GetID(),
		d.simulationID,
		now,
		d,
		"Driver Command",
		reflect.TypeOf(cmd).String(),
		nil,
	)
}

func (d *Driver) logCmdComplete(cmd Command, now sim.VTimeInSec) {
	tracing.EndTask(cmd.GetID(), now, d)
}

func (d *Driver) processNoopCommand(
	now sim.VTimeInSec,
	cmd *NoopCommand,
	queue *CommandQueue,
) bool {
	queue.Dequeue()
	return true
}

func (d *Driver) logTaskToGPUInitiate(
	now sim.VTimeInSec,
	cmd Command,
	req sim.Msg,
) {
	tracing.TraceReqInitiate(req, now, d, cmd.GetID())
}

func (d *Driver) logTaskToGPUClear(
	now sim.VTimeInSec,
	req sim.Msg,
) {
	tracing.TraceReqFinalize(req, now, d)
}

func (d *Driver) processLaunchKernelCommand(
	now sim.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) bool {
	dev := d.devices[queue.GPUID]
	if dev.Type == internal.DeviceTypeUnifiedGPU {
		return d.processUnifiedMultiGPULaunchKernelCommand(now, cmd, queue)
	}

	req := protocol.NewLaunchKernelReq(now,
		d.gpuPort, d.GPUs[queue.GPUID-1])
	req.PID = queue.Context.pid
	req.HsaCo = cmd.CodeObject

	req.Packet = cmd.Packet
	req.PacketAddress = uint64(cmd.DPacket)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)

	d.requestsToSend = append(d.requestsToSend, req)

	queue.Context.l2Dirty = true
	queue.Context.markAllBuffersDirty()

	d.logTaskToGPUInitiate(now, cmd, req)

	return true
}

func (d *Driver) processUnifiedMultiGPULaunchKernelCommand(
	now sim.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) bool {
	dev := d.devices[queue.GPUID]

	for i, gpuID := range dev.UnifiedGPUIDs {
		req := protocol.NewLaunchKernelReq(now,
			d.gpuPort, d.GPUs[gpuID-1])
		req.PID = queue.Context.pid
		req.HsaCo = cmd.CodeObject
		req.Packet = cmd.Packet
		req.PacketAddress = uint64(cmd.DPacket)

		numGPUs := len(dev.UnifiedGPUIDs)
		currentGPUIndex := i
		req.WGFilter = func(
			pkt *kernels.HsaKernelDispatchPacket,
			wg *kernels.WorkGroup,
		) bool {
			numWGX := (pkt.GridSizeX-1)/uint32(pkt.WorkgroupSizeX) + 1
			numWGY := (pkt.GridSizeY-1)/uint32(pkt.WorkgroupSizeY) + 1
			numWGZ := (pkt.GridSizeZ-1)/uint32(pkt.WorkgroupSizeZ) + 1
			numWG := int(numWGX * numWGY * numWGZ)

			flattenedID :=
				wg.IDZ*int(numWGX)*int(numWGY) +
					wg.IDY*int(numWGX) +
					wg.IDX

			wgPerGPU := (numWG-1)/numGPUs + 1

			return flattenedID/wgPerGPU == currentGPUIndex
		}

		queue.IsRunning = true
		cmd.Reqs = append(cmd.Reqs, req)

		d.requestsToSend = append(d.requestsToSend, req)

		queue.Context.l2Dirty = true
		queue.Context.markAllBuffersDirty()

		d.logTaskToGPUInitiate(now, cmd, req)
	}

	return true
}

func (d *Driver) processLaunchKernelReturn(
	now sim.VTimeInSec,
	req *protocol.LaunchKernelReq,
) bool {
	cmd, cmdQueue := d.findCommandByReq(req)
	cmd.RemoveReq(req)

	d.logTaskToGPUClear(now, req)

	if len(cmd.GetReqs()) == 0 {
		cmdQueue.IsRunning = false
		cmdQueue.Dequeue()

		d.logCmdComplete(cmd, now)
	}

	return true
}

func (d *Driver) findCommandByReq(req sim.Msg) (Command, *CommandQueue) {
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

func (d *Driver) parseFromMMU(now sim.VTimeInSec) bool {
	if d.isCurrentlyHandlingMigrationReq {
		return false
	}

	req := d.mmuPort.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *vm.PageMigrationReqToDriver:
		d.currentPageMigrationReq = req
		d.isCurrentlyHandlingMigrationReq = true
		d.initiateRDMADrain(now)
	default:
		log.Panicf("Driver cannot handle request of type %s",
			reflect.TypeOf(req))
	}

	return true
}

func (d *Driver) initiateRDMADrain(now sim.VTimeInSec) bool {
	for i := 0; i < len(d.GPUs); i++ {
		req := protocol.NewRDMADrainCmdFromDriver(now, d.gpuPort,
			d.GPUs[i])
		d.requestsToSend = append(d.requestsToSend, req)
		d.numRDMADrainACK++
	}

	return true
}

func (d *Driver) processRDMADrainRsp(
	now sim.VTimeInSec,
	req *protocol.RDMADrainRspToDriver,
) bool {
	d.numRDMADrainACK--

	if d.numRDMADrainACK == 0 {
		d.sendShootDownReqs(now)
	}

	return true
}

func (d *Driver) sendShootDownReqs(now sim.VTimeInSec) bool {
	vAddr := make([]uint64, 0)
	migrationInfo := d.currentPageMigrationReq.MigrationInfo

	numReqsGPUInMap := 0
	for i := 1; i < d.GetNumGPUs()+1; i++ {
		pages, found := migrationInfo.GPUReqToVAddrMap[uint64(i)]

		if found {
			numReqsGPUInMap++
			for j := 0; j < len(pages); j++ {
				vAddr = append(vAddr, pages[j])
			}
		}
	}

	accessingGPUs := d.currentPageMigrationReq.CurrAccessingGPUs
	pid := d.currentPageMigrationReq.PID
	d.numShootDownACK = uint64(len(accessingGPUs))

	for i := 0; i < len(accessingGPUs); i++ {
		toShootdownGPU := accessingGPUs[i] - 1
		shootDownReq := protocol.NewShootdownCommand(
			now,
			d.gpuPort, d.GPUs[toShootdownGPU],
			vAddr, pid)
		d.requestsToSend = append(d.requestsToSend, shootDownReq)
	}

	return true
}

func (d *Driver) processShootdownCompleteRsp(
	now sim.VTimeInSec,
	req *protocol.ShootDownCompleteRsp,
) bool {
	d.numShootDownACK--

	if d.numShootDownACK == 0 {
		toRequestFromGPU := d.currentPageMigrationReq.CurrPageHostGPU
		toRequestFromPMCPort := d.RemotePMCPorts[toRequestFromGPU-1]

		migrationInfo := d.currentPageMigrationReq.MigrationInfo

		requestingGPUs := d.findRequestingGPUs(migrationInfo)
		context := d.findContext(d.currentPageMigrationReq.PID)

		pageVaddrs := make(map[uint64][]uint64)

		for i := 0; i < len(requestingGPUs); i++ {
			pageVaddrs[requestingGPUs[i]] =
				migrationInfo.GPUReqToVAddrMap[requestingGPUs[i]+1]
		}

		for gpuID, vAddrs := range pageVaddrs {
			for i := 0; i < len(vAddrs); i++ {
				vAddr := vAddrs[i]
				page, oldPAddr :=
					d.preparePageForMigration(vAddr, context, gpuID)

				req := protocol.NewPageMigrationReqToCP(now, d.gpuPort,
					d.GPUs[gpuID])
				req.DestinationPMCPort = toRequestFromPMCPort
				req.ToReadFromPhysicalAddress = oldPAddr
				req.ToWriteToPhysicalAddress = page.PAddr
				req.PageSize = d.currentPageMigrationReq.PageSize

				d.migrationReqToSendToCP = append(d.migrationReqToSendToCP, req)
				d.numPagesMigratingACK++
			}
		}
		return true
	}

	return false
}

func (d *Driver) findRequestingGPUs(
	migrationInfo *vm.PageMigrationInfo,
) []uint64 {
	requestingGPUs := make([]uint64, 0)

	for i := 1; i < d.GetNumGPUs()+1; i++ {
		_, found := migrationInfo.GPUReqToVAddrMap[uint64(i)]
		if found {
			requestingGPUs = append(requestingGPUs, uint64(i-1))
		}
	}
	return requestingGPUs
}

func (d *Driver) findContext(pid ca.PID) *Context {
	context := &Context{}
	for i := 0; i < len(d.contexts); i++ {
		if d.contexts[i].pid == d.currentPageMigrationReq.PID {
			context = d.contexts[i]
		}
	}
	if context == nil {
		log.Panicf("Process does not exist")
	}
	return context
}

func (d *Driver) preparePageForMigration(
	vAddr uint64,
	context *Context,
	gpuID uint64,
) (*vm.Page, uint64) {
	page, found := d.pageTable.Find(context.pid, vAddr)
	if !found {
		panic("page not founds")
	}
	oldPAddr := page.PAddr

	newPage := d.memAllocator.AllocatePageWithGivenVAddr(
		context.pid, int(gpuID+1), vAddr, true)
	newPage.DeviceID = gpuID + 1

	newPage.IsMigrating = true
	d.pageTable.Update(newPage)

	return &newPage, oldPAddr
}

func (d *Driver) sendMigrationReqToCP(now sim.VTimeInSec) bool {
	if len(d.migrationReqToSendToCP) == 0 {
		return false
	}

	if d.isCurrentlyMigratingOnePage {
		return false
	}

	req := d.migrationReqToSendToCP[0]
	req.SendTime = now

	err := d.gpuPort.Send(req)
	if err == nil {
		d.migrationReqToSendToCP = d.migrationReqToSendToCP[1:]
		d.isCurrentlyMigratingOnePage = true
		return true
	}

	return false
}

func (d *Driver) processPageMigrationRspFromCP(
	now sim.VTimeInSec,
	rsp *protocol.PageMigrationRspToDriver,
) bool {
	d.numPagesMigratingACK--
	d.isCurrentlyMigratingOnePage = false

	if d.numPagesMigratingACK == 0 {
		d.prepareGPURestartReqs(now)
		d.preparePageMigrationRspToMMU(now)
	}

	return true
}

func (d *Driver) prepareGPURestartReqs(now sim.VTimeInSec) {
	accessingGPUs := d.currentPageMigrationReq.CurrAccessingGPUs

	for i := 0; i < len(accessingGPUs); i++ {
		restartGPUID := accessingGPUs[i] - 1
		restartReq := protocol.NewGPURestartReq(
			now,
			d.gpuPort,
			d.GPUs[restartGPUID])
		d.requestsToSend = append(d.requestsToSend, restartReq)
		d.numRestartACK++
	}
}

func (d *Driver) preparePageMigrationRspToMMU(now sim.VTimeInSec) {
	requestingGPUs := make([]uint64, 0)

	migrationInfo := d.currentPageMigrationReq.MigrationInfo

	for i := 1; i < d.GetNumGPUs()+1; i++ {
		_, found := migrationInfo.GPUReqToVAddrMap[uint64(i)]
		if found {
			requestingGPUs = append(requestingGPUs, uint64(i-1))
		}
	}

	pageVaddrs := make(map[uint64][]uint64)

	for i := 0; i < len(requestingGPUs); i++ {
		pageVaddrs[requestingGPUs[i]] = migrationInfo.GPUReqToVAddrMap[requestingGPUs[i]+1]
	}

	req := vm.NewPageMigrationRspFromDriver(now, d.mmuPort,
		d.currentPageMigrationReq.Src)

	for _, vAddrs := range pageVaddrs {
		for j := 0; j < len(vAddrs); j++ {
			req.VAddr = append(req.VAddr, vAddrs[j])
		}
	}
	req.RspToTop = d.currentPageMigrationReq.RespondToTop
	d.toSendToMMU = req
}

func (d *Driver) handleGPURestartRsp(
	now sim.VTimeInSec,
	req *protocol.GPURestartRsp,
) bool {
	d.numRestartACK--
	if d.numRestartACK == 0 {
		d.prepareRDMARestartReqs(now)
	}
	return true
}

func (d *Driver) prepareRDMARestartReqs(now sim.VTimeInSec) {
	for i := 0; i < len(d.GPUs); i++ {
		req := protocol.NewRDMARestartCmdFromDriver(now,
			d.gpuPort, d.GPUs[i])
		d.requestsToSend = append(d.requestsToSend, req)
		d.numRDMARestartACK++
	}
}

func (d *Driver) processRDMARestartRspToDriver(
	now sim.VTimeInSec,
	rsp *protocol.RDMARestartRspToDriver) bool {
	d.numRDMARestartACK--

	if d.numRDMARestartACK == 0 {
		d.currentPageMigrationReq = nil
		d.isCurrentlyHandlingMigrationReq = false
		return true
	}
	return true
}

func (d *Driver) sendToMMU(now sim.VTimeInSec) bool {
	if d.toSendToMMU == nil {
		return false
	}
	req := d.toSendToMMU
	req.SendTime = now
	err := d.mmuPort.Send(req)
	if err == nil {
		d.toSendToMMU = nil
		return true
	}

	return false
}
