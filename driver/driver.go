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
	d.sendToMMU(now)
	d.sendMigrationReqToCP(now)
	d.processReturnReq(now)
	d.processNewCommand(now)
	d.parseFromMMU(now)

	if d.NeedTick {
		d.TickLater(now)
	}
}

func (d *Driver) sendToGPUs(now akita.VTimeInSec) {
	if len(d.requestsToSend) == 0 {
		return
	}

	req := d.requestsToSend[0]
	req.Meta().SendTime = now
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
	case *gcn3.RDMADrainRspToDriver:
		d.processRDMADrainRsp(now, req)
	case *gcn3.ShootDownCompleteRsp:
		d.processShootdownCompleteRsp(now, req)
	case *gcn3.PageMigrationRspToDriver:
		d.processPageMigrationRspFromCP(now, req)
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
) {
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

func (d *Driver) parseFromMMU(now akita.VTimeInSec) {
	if d.isCurrentlyHandlingMigrationReq {
		return
	}
	req := d.ToMMU.Retrieve(now)

	if req == nil {
		return
	}

	switch req := req.(type) {
	case *vm.PageMigrationReqToDriver:
		d.currentPageMigrationReq = req
		d.isCurrentlyHandlingMigrationReq = true
		d.initiateRDMADrain(now)
	default:
		log.Panicf("Driver canot handle request of type %s", reflect.TypeOf(req))
	}
	d.NeedTick = true

}

func (d *Driver) initiateRDMADrain(now akita.VTimeInSec) {
	for i := 0; i < len(d.GPUs); i++ {
		req := gcn3.NewRDMADrainCmdFromDriver(now, d.ToGPUs, d.GPUs[i].ToDriver)
		err := d.ToGPUs.Send(req)
		d.numRDMADrainACK++
		if err != nil {
			log.Panicf("Failed to send RDMA Drain reqs to all GPUs")
		}
	}
	d.NeedTick = true

}

func (d *Driver) processRDMADrainRsp(now akita.VTimeInSec, req *gcn3.RDMADrainRspToDriver) {
	d.numRDMADrainACK--

	if d.numRDMADrainACK == 0 {
		d.sendShootDownReqs(now)
	}
}

func (d *Driver) sendShootDownReqs(now akita.VTimeInSec) {
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
		shootDwnReq := gcn3.NewShootdownCommand(now, d.ToGPUs, d.GPUs[toShootdownGPU].ToDriver, vAddr, pid)
		d.requestsToSend = append(d.requestsToSend, shootDwnReq)
	}

	d.NeedTick = true

}

func (d *Driver) processShootdownCompleteRsp(now akita.VTimeInSec, req *gcn3.ShootDownCompleteRsp) {
	d.numShootDownACK--

	if d.numShootDownACK == 0 {
		toRequestFromGPU := d.currentPageMigrationReq.CurrPageHostGPU
		toRequestFromPMEPort := d.RemotePMCPorts[toRequestFromGPU-1]

		migrationInfo := d.currentPageMigrationReq.MigrationInfo

		requestingGPUs := d.findRequestingGPUs(migrationInfo)
		context := d.findContext(d.currentPageMigrationReq.PID)

		pageVaddrs := make(map[uint64][]uint64)

		for i := 0; i < len(requestingGPUs); i++ {
			pageVaddrs[requestingGPUs[i]] = migrationInfo.GpuReqToVAddrMap[uint64(requestingGPUs[i]+1)]

		}

		for gpuID, vAddrs := range pageVaddrs {
			for i := 0; i < len(vAddrs); i++ {
				vAddr := vAddrs[i]
				page, oldPaddr := d.preparePageForMigration(vAddr, context, gpuID)

				req := gcn3.NewPageMigrationReqToCP(now, d.ToGPUs, d.GPUs[gpuID].ToDriver)
				req.DestinationPMCPort = toRequestFromPMEPort
				req.ToReadFromPhysicalAddress = oldPaddr
				req.ToWriteToPhysicalAddress = page.PAddr
				req.PageSize = d.currentPageMigrationReq.PageSize

				d.migrationReqToSendToCP = append(d.migrationReqToSendToCP, req)
				d.numPagesMigratingACK++

			}
		}
	}
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
	newPage.GPUID = uint64(gpuID + 1)

	d.MMU.MarkPageAsMigrating(vAddr, d.currentPageMigrationReq.PID)

	return &newPage, oldPaddr

}

func (d *Driver) sendMigrationReqToCP(now akita.VTimeInSec) {
	if len(d.migrationReqToSendToCP) == 0 {
		return
	}

	if d.isCurrentlyMigratingOnePage {
		return
	}

	req := d.migrationReqToSendToCP[0]
	req.SendTime = now

	err := d.ToGPUs.Send(req)

	if err == nil {
		d.migrationReqToSendToCP = d.migrationReqToSendToCP[1:]
		d.isCurrentlyMigratingOnePage = true
		d.NeedTick = true
	}
}

func (d *Driver) processPageMigrationRspFromCP(
	now akita.VTimeInSec,
	rsp *gcn3.PageMigrationRspToDriver,
) {

	d.numPagesMigratingACK--
	d.isCurrentlyMigratingOnePage = false

	if d.numPagesMigratingACK == 0 {
		d.prepareRDMARestartReqs(now)
		d.prepareGPURestartReqs(now)
		d.preparePageMigrationRspToMMU(now)
	}
	d.NeedTick = true
}

func (d *Driver) prepareRDMARestartReqs(now akita.VTimeInSec) {
	for i := 0; i < len(d.GPUs); i++ {
		req := gcn3.NewRDMARestartCmdFromDriver(now, d.ToGPUs, d.GPUs[i].ToDriver)
		d.requestsToSend = append(d.requestsToSend, req)
	}
}

func (d *Driver) prepareGPURestartReqs(now akita.VTimeInSec) {
	accessingGPUs := d.currentPageMigrationReq.CurrAccessingGPUs

	for i := 0; i < len(accessingGPUs); i++ {
		restartGPUID := accessingGPUs[i] - 1
		restartReq := gcn3.NewGPURestartReq(now, d.ToGPUs, d.GPUs[restartGPUID].ToDriver)
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
		pageVaddrs[requestingGPUs[i]] = migrationInfo.GpuReqToVAddrMap[uint64(requestingGPUs[i]+1)]
	}

	req := vm.NewPageMigrationRspFromDriver(now, d.ToMMU, d.currentPageMigrationReq.Src)

	for _, vAddrs := range pageVaddrs {
		for j := 0; j < len(vAddrs); j++ {
			req.VAddr = append(req.VAddr, vAddrs[j])
		}
	}
	req.RspToTop = d.currentPageMigrationReq.RespondToTop
	d.toSendToMMU = req
	d.currentPageMigrationReq = nil
}

func (d *Driver) sendToMMU(now akita.VTimeInSec) {
	if d.toSendToMMU == nil {
		return
	}
	err := d.ToMMU.Send(d.toSendToMMU)
	if err == nil {
		d.NeedTick = true
		d.toSendToMMU = nil
	}
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine, mmu mmu.MMU) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	memAllocatorImpl := internal.NewMemoryAllocator(mmu, 12)
	driver.memAllocator = memAllocatorImpl

	distributorImpl := newDistributorImpl(memAllocatorImpl)
	distributorImpl.pageSizeAsPowerOf2 = 12
	driver.distributor = distributorImpl

	driver.MMU = mmu

	driver.ToGPUs = akita.NewLimitNumMsgPort(driver, 40960000)
	driver.ToMMU = akita.NewLimitNumMsgPort(driver, 1)

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)

	driver.reserveMemoryForCPU()

	return driver
}
