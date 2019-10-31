package driver

import (
	"bytes"
	"encoding/binary"
	"log"
	"reflect"
	"sync"

	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver/internal"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/util/ca"
	"gitlab.com/akita/util/tracing"
)

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.TickingComponent

	memAllocator internal.MemoryAllocator
	distributor  distributor

	GPUs []*gcn3.GPU
	MMU  mmu.MMU

	ToMMU akita.Port

	requestsToSend []akita.Msg

	contextMutex sync.Mutex
	contexts     []*Context

	ToGPUs akita.Port

	driverStopped chan bool
	enqueueSignal chan bool
	engineMutex   sync.Mutex
	simulationID  string

	Log2PageSize uint64

	currentPageMigrationReq *vm.PageMigrationReqToDriver
	toSendToMMU             *vm.PageMigrationRspFromDriver
	migrationReqToSendToCP  []*gcn3.PageMigrationReqToCP

	isCurrentlyHandlingMigrationReq bool
	numRDMADrainACK                 uint64
	numShootDownACK                 uint64
	numRestartACK                   uint64
	numPagesMigratingACK            uint64
	isCurrentlyMigratingOnePage     bool

	RemotePMCPorts []akita.Port
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
			d.runEngine()
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

func (d *Driver) Tick(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = d.sendToGPUs(now) || madeProgress
	madeProgress = d.sendToMMU(now) || madeProgress
	madeProgress = d.sendMigrationReqToCP(now) || madeProgress
	madeProgress = d.processReturnReq(now) || madeProgress
	madeProgress = d.processNewCommand(now) || madeProgress
	madeProgress = d.parseFromMMU(now) || madeProgress

	return madeProgress
}

func (d *Driver) sendToGPUs(now akita.VTimeInSec) bool {
	if len(d.requestsToSend) == 0 {
		return false
	}

	req := d.requestsToSend[0]
	req.Meta().SendTime = now
	err := d.ToGPUs.Send(req)
	if err == nil {
		d.requestsToSend = d.requestsToSend[1:]
		return true
	}

	return false
}

//nolint:gocyclo
func (d *Driver) processReturnReq(now akita.VTimeInSec) bool {
	req := d.ToGPUs.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *gcn3.MemCopyH2DReq:
		return d.processMemCopyH2DReturn(now, req)
	case *gcn3.MemCopyD2HReq:
		return d.processMemCopyD2HReturn(now, req)
	case *gcn3.LaunchKernelReq:
		return d.processLaunchKernelReturn(now, req)
	case *gcn3.FlushCommand:
		return d.processFlushReturn(now, req)
	case *gcn3.RDMADrainRspToDriver:
		return d.processRDMADrainRsp(now, req)
	case *gcn3.ShootDownCompleteRsp:
		return d.processShootdownCompleteRsp(now, req)
	case *gcn3.PageMigrationRspToDriver:
		return d.processPageMigrationRspFromCP(now, req)
	case *gcn3.GPURestartRsp:
		return d.handleGPURestartRsp(now, req)
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}

	panic("never")
}

func (d *Driver) processNewCommand(now akita.VTimeInSec) bool {
	madeProgress := false

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

			madeProgress = d.processOneCommand(now, q) || madeProgress
		}
		context.queueMutex.Unlock()
	}
	d.contextMutex.Unlock()

	return madeProgress
}

func (d *Driver) processOneCommand(
	now akita.VTimeInSec,
	cmdQueue *CommandQueue,
) bool {
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
	return true
}

func (d *Driver) logCmdStart(cmd Command, now akita.VTimeInSec) {
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

func (d *Driver) logCmdComplete(cmd Command, now akita.VTimeInSec) {
	tracing.EndTask(cmd.GetID(), now, d)
}

func (d *Driver) processNoopCommand(
	now akita.VTimeInSec,
	cmd *NoopCommand,
	queue *CommandQueue,
) bool {
	queue.Dequeue()
	return true
}

func (d *Driver) processMemCopyH2DCommand(
	now akita.VTimeInSec,
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) bool {
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
			d.ToGPUs, d.GPUs[gpuID-1].CommandProcessor.ToDriver,
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
	return true
}

func (d *Driver) logTaskToGPUInitiate(
	now akita.VTimeInSec,
	cmd Command,
	req akita.Msg,
) {
	tracing.TraceReqInitiate(req, now, d, cmd.GetID())
}

func (d *Driver) logTaskToGPUClear(
	now akita.VTimeInSec,
	req akita.Msg,
) {
	tracing.TraceReqFinalize(req, now, d)
}

func (d *Driver) processMemCopyH2DReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyH2DReq,
) bool {
	d.logTaskToGPUClear(now, req)

	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyH2DCommand)
	newReqs := make([]akita.Msg, 0, len(copyCmd.Reqs)-1)
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

	return true
}

func (d *Driver) processMemCopyD2HCommand(
	now akita.VTimeInSec,
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) bool {
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
			d.ToGPUs, d.GPUs[gpuID-1].CommandProcessor.ToDriver,
			pAddr, cmd.RawData[offset:offset+sizeToCopy])
		cmd.Reqs = append(cmd.Reqs, req)
		d.requestsToSend = append(d.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		d.logTaskToGPUInitiate(now, cmd, req)
	}

	queue.IsRunning = true
	return true
}

func (d *Driver) processMemCopyD2HReturn(
	now akita.VTimeInSec,
	req *gcn3.MemCopyD2HReq,
) bool {
	d.logTaskToGPUClear(now, req)

	cmd, cmdQueue := d.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyD2HCommand)
	newReqs := make([]akita.Msg, 0, len(copyCmd.Reqs)-1)
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

	return true
}

func (d *Driver) processLaunchKernelCommand(
	now akita.VTimeInSec,
	cmd *LaunchKernelCommand,
	queue *CommandQueue,
) bool {
	req := gcn3.NewLaunchKernelReq(now,
		d.ToGPUs, d.GPUs[queue.GPUID-1].CommandProcessor.ToDriver)
	req.PID = queue.Context.pid
	req.HsaCo = cmd.CodeObject

	req.Packet = cmd.Packet
	req.PacketAddress = uint64(cmd.DPacket)

	queue.IsRunning = true
	cmd.Reqs = append(cmd.Reqs, req)

	d.requestsToSend = append(d.requestsToSend, req)

	queue.Context.l2Dirty = true

	d.logCmdStart(cmd, now)
	d.logTaskToGPUInitiate(now, cmd, req)

	return true
}

func (d *Driver) processLaunchKernelReturn(
	now akita.VTimeInSec,
	req *gcn3.LaunchKernelReq,
) bool {
	cmd, cmdQueue := d.findCommandByReq(req)
	cmdQueue.IsRunning = false
	cmdQueue.Dequeue()

	d.logTaskToGPUClear(now, req)
	d.logCmdComplete(cmd, now)

	return true
}

func (d *Driver) processFlushCommand(
	now akita.VTimeInSec,
	cmd *FlushCommand,
	queue *CommandQueue,
) bool {
	for _, gpu := range d.GPUs {
		req := gcn3.NewFlushCommand(now,
			d.ToGPUs, gpu.CommandProcessor.ToDriver)
		d.requestsToSend = append(d.requestsToSend, req)
		cmd.Reqs = append(cmd.Reqs, req)
		d.logTaskToGPUInitiate(now, cmd, req)
	}

	queue.IsRunning = true

	d.logCmdStart(cmd, now)

	return true
}

func (d *Driver) processFlushReturn(
	now akita.VTimeInSec,
	req *gcn3.FlushCommand,
) bool {
	d.logTaskToGPUClear(now, req)

	cmd, cmdQueue := d.findCommandByReq(req)
	flushCmd := cmd.(*FlushCommand)

	newReqs := make([]akita.Msg, 0)
	for _, r := range flushCmd.Reqs {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	flushCmd.Reqs = newReqs

	if len(flushCmd.Reqs) > 0 {
		return true
	}

	cmdQueue.IsRunning = false
	cmdQueue.Dequeue()

	cmdQueue.Context.l2Dirty = false

	d.logCmdComplete(cmd, now)

	return true
}

func (d *Driver) findCommandByReq(req akita.Msg) (Command, *CommandQueue) {
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

func (d *Driver) parseFromMMU(now akita.VTimeInSec) bool {
	if d.isCurrentlyHandlingMigrationReq {
		return false
	}

	req := d.ToMMU.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *vm.PageMigrationReqToDriver:
		d.currentPageMigrationReq = req
		d.isCurrentlyHandlingMigrationReq = true
		d.initiateRDMADrain(now)
	default:
		log.Panicf("Driver canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *Driver) initiateRDMADrain(now akita.VTimeInSec) bool {
	for i := 0; i < len(d.GPUs); i++ {
		req := gcn3.NewRDMADrainCmdFromDriver(now, d.ToGPUs,
			d.GPUs[i].CommandProcessor.ToDriver)
		err := d.ToGPUs.Send(req)
		d.numRDMADrainACK++
		if err != nil {
			log.Panicf("Failed to send RDMA Drain reqs to all GPUs")
		}
	}

	return true
}

func (d *Driver) processRDMADrainRsp(
	now akita.VTimeInSec,
	req *gcn3.RDMADrainRspToDriver,
) bool {
	d.numRDMADrainACK--

	if d.numRDMADrainACK == 0 {
		d.sendShootDownReqs(now)
	}

	return true
}

func (d *Driver) sendShootDownReqs(now akita.VTimeInSec) bool {
	vAddr := make([]uint64, 0)
	migrationInfo := d.currentPageMigrationReq.MigrationInfo

	numReqsGPUInMap := 0
	for i := 1; i < d.GetNumGPUs()+1; i++ {
		pages, found := migrationInfo.GpuReqToVAddrMap[uint64(i)]

		if found {
			numReqsGPUInMap++
			for j := 0; j < len(pages); j++ {
				vAddr = append(vAddr, pages[j])
			}
		}
	}

	accesingGPUs := d.currentPageMigrationReq.CurrAccessingGPUs
	pid := d.currentPageMigrationReq.PID
	d.numShootDownACK = uint64(len(accesingGPUs))

	for i := 0; i < len(accesingGPUs); i++ {
		toShootdownGPU := accesingGPUs[i] - 1
		shootDwnReq := gcn3.NewShootdownCommand(now, d.ToGPUs, d.GPUs[toShootdownGPU].CommandProcessor.ToDriver, vAddr, pid)
		d.requestsToSend = append(d.requestsToSend, shootDwnReq)
	}

	return true
}

func (d *Driver) processShootdownCompleteRsp(
	now akita.VTimeInSec,
	req *gcn3.ShootDownCompleteRsp,
) bool {
	d.numShootDownACK--

	if d.numShootDownACK == 0 {
		toRequestFromGPU := d.currentPageMigrationReq.CurrPageHostGPU
		toRequestFromPMEPort := d.RemotePMCPorts[toRequestFromGPU-1]

		migrationInfo := d.currentPageMigrationReq.MigrationInfo

		requestingGPUs := d.findRequestingGPUs(migrationInfo)
		context := d.findContext(d.currentPageMigrationReq.PID)

		pageVaddrs := make(map[uint64][]uint64)

		for i := 0; i < len(requestingGPUs); i++ {
			pageVaddrs[requestingGPUs[i]] = migrationInfo.GpuReqToVAddrMap[requestingGPUs[i]+1]
		}

		for gpuID, vAddrs := range pageVaddrs {
			for i := 0; i < len(vAddrs); i++ {
				vAddr := vAddrs[i]
				page, oldPaddr := d.preparePageForMigration(vAddr, context, gpuID)

				req := gcn3.NewPageMigrationReqToCP(now, d.ToGPUs,
					d.GPUs[gpuID].CommandProcessor.ToDriver)
				req.DestinationPMCPort = toRequestFromPMEPort
				req.ToReadFromPhysicalAddress = oldPaddr
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

func (d *Driver) findRequestingGPUs(migrationInfo *vm.PageMigrationInfo) []uint64 {
	requestingGPUs := make([]uint64, 0)

	for i := 1; i < d.GetNumGPUs()+1; i++ {
		_, found := migrationInfo.GpuReqToVAddrMap[uint64(i)]
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

func (d *Driver) preparePageForMigration(vAddr uint64, context *Context, gpuID uint64) (*vm.Page, uint64) {
	page := d.MMU.GetPageWithGivenVAddr(vAddr, context.pid)
	oldPaddr := page.PAddr

	d.memAllocator.RemovePage(vAddr)

	newPage := d.memAllocator.AllocatePageWithGivenVAddr(context.pid, int(gpuID+1), vAddr, true)
	newPage.GPUID = gpuID + 1

	d.MMU.MarkPageAsMigrating(vAddr, d.currentPageMigrationReq.PID)

	return &newPage, oldPaddr
}

func (d *Driver) sendMigrationReqToCP(now akita.VTimeInSec) bool {
	if len(d.migrationReqToSendToCP) == 0 {
		return false
	}

	if d.isCurrentlyMigratingOnePage {
		return false
	}

	req := d.migrationReqToSendToCP[0]
	req.SendTime = now

	err := d.ToGPUs.Send(req)
	if err == nil {
		d.migrationReqToSendToCP = d.migrationReqToSendToCP[1:]
		d.isCurrentlyMigratingOnePage = true
		return true
	}

	return false
}

func (d *Driver) processPageMigrationRspFromCP(
	now akita.VTimeInSec,
	rsp *gcn3.PageMigrationRspToDriver,
) bool {
	d.numPagesMigratingACK--
	d.isCurrentlyMigratingOnePage = false

	if d.numPagesMigratingACK == 0 {
		d.prepareRDMARestartReqs(now)
		d.prepareGPURestartReqs(now)
		d.preparePageMigrationRspToMMU(now)
	}

	return true
}

func (d *Driver) prepareRDMARestartReqs(now akita.VTimeInSec) {
	for i := 0; i < len(d.GPUs); i++ {
		req := gcn3.NewRDMARestartCmdFromDriver(now, d.ToGPUs,
			d.GPUs[i].CommandProcessor.ToDriver)
		d.requestsToSend = append(d.requestsToSend, req)
	}
}

func (d *Driver) prepareGPURestartReqs(now akita.VTimeInSec) {
	accessingGPUs := d.currentPageMigrationReq.CurrAccessingGPUs

	for i := 0; i < len(accessingGPUs); i++ {
		restartGPUID := accessingGPUs[i] - 1
		restartReq := gcn3.NewGPURestartReq(
			now, d.ToGPUs, d.GPUs[restartGPUID].CommandProcessor.ToDriver)
		d.requestsToSend = append(d.requestsToSend, restartReq)
		d.numRestartACK++
	}
}

func (d *Driver) preparePageMigrationRspToMMU(now akita.VTimeInSec) {
	requestingGPUs := make([]uint64, 0)

	migrationInfo := d.currentPageMigrationReq.MigrationInfo

	for i := 1; i < d.GetNumGPUs()+1; i++ {
		_, found := migrationInfo.GpuReqToVAddrMap[uint64(i)]
		if found {
			requestingGPUs = append(requestingGPUs, uint64(i-1))
		}
	}

	pageVaddrs := make(map[uint64][]uint64)

	for i := 0; i < len(requestingGPUs); i++ {
		pageVaddrs[requestingGPUs[i]] = migrationInfo.GpuReqToVAddrMap[requestingGPUs[i]+1]
	}

	req := vm.NewPageMigrationRspFromDriver(now, d.ToMMU, d.currentPageMigrationReq.Src)

	for _, vAddrs := range pageVaddrs {
		for j := 0; j < len(vAddrs); j++ {
			req.VAddr = append(req.VAddr, vAddrs[j])
		}
	}
	req.RspToTop = d.currentPageMigrationReq.RespondToTop
	d.toSendToMMU = req
}

func (d *Driver) handleGPURestartRsp(
	now akita.VTimeInSec,
	req *gcn3.GPURestartRsp,
) bool {
	d.numRestartACK--
	if d.numRestartACK == 0 {
		d.currentPageMigrationReq = nil
		d.isCurrentlyHandlingMigrationReq = false
	}
	return true
}

func (d *Driver) sendToMMU(now akita.VTimeInSec) bool {
	if d.toSendToMMU == nil {
		return false
	}
	err := d.ToMMU.Send(d.toSendToMMU)
	if err == nil {
		d.toSendToMMU = nil
		return true
	}

	return false
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine, mmu mmu.MMU, log2PageSize uint64) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	driver.Log2PageSize = log2PageSize

	memAllocatorImpl := internal.NewMemoryAllocator(mmu, log2PageSize)
	driver.memAllocator = memAllocatorImpl

	distributorImpl := newDistributorImpl(memAllocatorImpl)
	distributorImpl.pageSizeAsPowerOf2 = log2PageSize
	driver.distributor = distributorImpl

	driver.MMU = mmu

	driver.ToGPUs = akita.NewLimitNumMsgPort(driver, 40960000, "driver.ToGPUs")
	driver.ToMMU = akita.NewLimitNumMsgPort(driver, 1, "driver.ToMMU")

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)

	driver.reserveMemoryForCPU()

	return driver
}
