package gpu

import (
	"encoding/binary"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type GPUController struct {
	*sim.TickingComponent

	ID      string
	gpuName string

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSMs   sim.Port
	SMs     map[string]*sm.SMController
	freeSMs []*sm.SMController

	ToSMSPs sim.Port

	// cache updates
	ToCaches sim.Port // used to send cache reqs
	ToDRAM   sim.Port // remote DRAM's port

	ToSMSPsMem sim.Port // used to receive and send mem reqs to SMSPs

	// toSMMem       sim.Port
	// toSMMemRemote sim.Port

	// toDRAM       sim.Port
	// toDRAMRemote sim.Port

	// L2Caches    []*writeback.Comp
	// L2CacheSize uint64
	// Drams    []*dram.Comp
	// DramSize uint64

	// PendingReadReq  map[string]*mem.ReadReq
	// PendingWriteReq map[string]*mem.WriteReq

	PendingSMSPtoGPUControllerMemReadReq  map[string]*message.SMSPToGPUControllerMemReadMsg
	PendingSMSPtoGPUControllerMemWriteReq map[string]*message.SMSPToGPUControllerMemWriteMsg

	PendingCacheReadReq  map[string]*message.GPUControllerToCachesMemReadMsg
	PendingCacheWriteReq map[string]*message.GPUControllerToCachesMemWriteMsg

	RDMAEngine *rdma.Comp

	undispatchedThreadblocks    []*trace.ThreadblockTrace
	unfinishedThreadblocksCount uint64

	finishedKernelsCount uint64
}

func (g *GPUController) SetDriverRemotePort(remote sim.Port) {
	g.toDriverRemote = remote
}

func (g *GPUController) Tick() bool {
	madeProgress := false
	madeProgress = g.reportFinishedKernels() || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs() || madeProgress
	madeProgress = g.processDriverInput() || madeProgress
	madeProgress = g.processSMsInput() || madeProgress
	madeProgress = g.processCaches() || madeProgress
	// madeProgress = g.processSMsInputMem() || madeProgress
	// madeProgress = g.processDRAMRsp() || madeProgress

	return madeProgress
}

func (g *GPUController) processCaches() bool {
	madeProgress := false
	madeProgress = g.processSMSPsRequestMem() || madeProgress
	madeProgress = g.processCachesReturnMem() || madeProgress
	return madeProgress
}

func (g *GPUController) processDriverInput() bool {
	msg := g.toDriver.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DriverToDeviceMsg:
		g.processDriverMsg(msg)
	default:
		log.WithField("function", "processDriverInput").Panic("Unhandled message type")
	}

	return true
}

func (g *GPUController) processSMsInput() bool {
	msg := g.toSMs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToDeviceMsg:
		g.processSMsMsg(msg)
	default:
		log.WithField("function", "processSMsInput").Panic("Unhandled message type")
	}

	return true
}

func (g *GPUController) processSMSPsRequestMem() bool {
	msg := g.ToSMSPsMem.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMSPToGPUControllerMemReadMsg:
		g.processSMSPToGPUControllerMemReadMsg(msg)
	case *message.SMSPToGPUControllerMemWriteMsg:
		g.processSMSPToGPUControllerMemWriteMsg(msg)
	default:
		log.WithField("function", "processSMSPsRequestMem").Panic("Unhandled message type")
	}

	return true
}

func (g *GPUController) processCachesReturnMem() bool {
	msg := g.ToCaches.PeekIncoming()
	if msg == nil {
		return false
	}
	// fmt.Printf("%T\n", msg)
	switch msg := msg.(type) {
	case *mem.WriteDoneRsp:
		write := g.PendingCacheWriteReq[msg.RespondTo]
		// fmt.Printf("%.10f, GPUController received msg from Caches, original ID = %s write complete\n",
		// 	g.CurrentTime(), write.OriginalSMSPtoGPUControllerID)

		g.processCachesWriteRspMsg(msg, write.OriginalSMSPtoGPUControllerID)
		// delete(g.PendingCacheWriteReq, msg.RespondTo)
		g.ToCaches.RetrieveIncoming()

		return true
	case *mem.DataReadyRsp:
		read := g.PendingCacheReadReq[msg.RespondTo]
		// delete(g.PendingReadReq, msg.RespondTo)

		// fmt.Printf("%.10f, GPUController received msg from Caches, original ID = %s read complete, %v\n",
		// 	g.CurrentTime(), read.OriginalSMSPtoGPUControllerID, msg.Data)

		g.processCachesReadRspMsg(msg, read.OriginalSMSPtoGPUControllerID)
		// delete(g.PendingCacheReadReq, msg.RespondTo)
		g.ToCaches.RetrieveIncoming()

		// g.toDRAM.RetrieveIncoming()

		return true
	default:
		log.Panicf("cannot process message of type %s", reflect.TypeOf(msg))
	}

	return false
}

func (g *GPUController) processDriverMsg(msg *message.DriverToDeviceMsg) {
	for i := range msg.Kernel.Threadblocks {
		g.undispatchedThreadblocks = append(g.undispatchedThreadblocks, msg.Kernel.Threadblocks[i])
		g.unfinishedThreadblocksCount++
	}
	g.toDriver.RetrieveIncoming()
}

func (g *GPUController) processSMsMsg(msg *message.SMToDeviceMsg) {
	if msg.ThreadblockFinished {
		g.freeSMs = append(g.freeSMs, g.SMs[msg.SMID])
		g.unfinishedThreadblocksCount--
		if g.unfinishedThreadblocksCount == 0 {
			g.finishedKernelsCount++
		}
	}
	g.toSMs.RetrieveIncoming()
}

func (g *GPUController) processSMSPToGPUControllerMemReadMsg(msg *message.SMSPToGPUControllerMemReadMsg) bool {
	// fmt.Printf("%.10f, %s, GPUController receives SMSPToGPUControllerMemReadMsg, read from address = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address)
	readReq := mem.ReadReqBuilder{}.
		WithSrc(g.ToCaches.AsRemote()).
		WithDst(g.ToDRAM.AsRemote()).
		WithAddress(msg.Address).
		WithPID(1).
		Build()
	err := g.ToCaches.Send(readReq)
	if err != nil {
		fmt.Printf("GPUController failed to send read mem request: %v\n", err)
		g.ToSMSPsMem.RetrieveIncoming()
		return false
	}
	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n",
	// 	g.Engine.CurrentTime(), msg.Address, readReq.ID)
	g.PendingSMSPtoGPUControllerMemReadReq[msg.ID] = msg
	g.PendingCacheReadReq[readReq.ID] = &message.GPUControllerToCachesMemReadMsg{
		OriginalSMSPtoGPUControllerID: msg.ID,
		Msg:                           *readReq,
	}
	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, readReq.ID)
	g.ToSMSPsMem.RetrieveIncoming()
	return true
}

func (g *GPUController) processSMSPToGPUControllerMemWriteMsg(msg *message.SMSPToGPUControllerMemWriteMsg) bool {
	// fmt.Printf("%.10f, %s, GPUController receives SMSPToGPUControllerMemWriteMsg, write to address = %d, data = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address, msg.Data)
	writeReq := mem.WriteReqBuilder{}.
		WithSrc(g.ToCaches.AsRemote()).
		WithDst(g.ToDRAM.AsRemote()).
		WithAddress(msg.Address).
		WithPID(1).
		WithData(uint32ToBytes(msg.Data)).
		Build()

	err := g.ToCaches.Send(writeReq)
	if err != nil {
		fmt.Printf("GPUController failed to send write mem request: %v\n", err)
		g.ToSMSPsMem.RetrieveIncoming()
		return false
	}
	g.PendingSMSPtoGPUControllerMemWriteReq[msg.ID] = msg
	g.PendingCacheWriteReq[writeReq.ID] = &message.GPUControllerToCachesMemWriteMsg{
		OriginalSMSPtoGPUControllerID: msg.ID,
		Msg:                           *writeReq,
	}

	// fmt.Printf("%.10f, GPUController, write request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, writeReq.ID)
	g.ToSMSPsMem.RetrieveIncoming()
	return true
}

func (g *GPUController) processCachesReadRspMsg(rspMsg *mem.DataReadyRsp, originalID string) bool {
	// fmt.Printf("%.10f, %s, GPUController is sending read rsp back to SMSP\n", g.Engine.CurrentTime(), g.Name())
	originalSMSPToGPUControllerReq := g.PendingSMSPtoGPUControllerMemReadReq[originalID]
	msg := &message.CachesToSMSPMemReadRspMsg{
		OriginalSMSPtoGPUControllerID: originalID,
		Msg:                           *rspMsg,
	}
	msg.Src = g.ToSMSPsMem.AsRemote()
	msg.Dst = originalSMSPToGPUControllerReq.Src
	msg.ID = sim.GetIDGenerator().Generate()
	err := g.ToSMSPsMem.Send(msg)
	if err != nil {
		fmt.Printf("GPUController failed to send read rsp back to SMSP: %v\n", err)
		return false
	}
	return true
}

func (g *GPUController) processCachesWriteRspMsg(rspMsg *mem.WriteDoneRsp, originalID string) bool {
	// fmt.Printf("%.10f, %s, GPUController is sending write rsp back to SMSP\n", g.Engine.CurrentTime(), g.Name())
	originalSMSPToGPUControllerReq := g.PendingSMSPtoGPUControllerMemWriteReq[originalID]
	msg := &message.CachesToSMSPMemWriteRspMsg{
		OriginalSMSPtoGPUControllerID: originalID,
		Msg:                           *rspMsg,
	}
	msg.Src = g.ToSMSPsMem.AsRemote()
	msg.Dst = originalSMSPToGPUControllerReq.Src
	msg.ID = sim.GetIDGenerator().Generate()
	err := g.ToSMSPsMem.Send(msg)
	if err != nil {
		fmt.Printf("GPUController failed to send write rsp back to SMSP: %v\n", err)
		return false
	}
	return true
}

// func (g *GPUController) processDRAMWriteRspMsg(rspMsg *mem.WriteDoneRsp) bool {
// 	fmt.Printf("%.10f, %s, GPUController\n", g.Engine.CurrentTime(), g.Name())
// 	msg := &message.GPUtoSMMemWriteMsg{
// 		Address: g.PendingReadReq[rspMsg.RespondTo].Address,
// 		Rsp:     *rspMsg,
// 	}
// 	msg.Src = g.toSMMem.AsRemote()
// 	msg.Dst = g.PendingReadReq[rspMsg.RespondTo].Src
// 	// err := g.toSMMem.Send(msg)
// 	// if err != nil {
// 	// 	fmt.Printf("GPUController failed to send write rsp to SMController: %v\n", err)
// 	// 	g.toDRAM.RetrieveIncoming()
// 	// 	return false
// 	// }
// 	g.toDRAM.RetrieveIncoming()
// 	return true
// }

func (g *GPUController) reportFinishedKernels() bool {
	if g.finishedKernelsCount == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		KernelFinished: true,
		DeviceID:       g.ID,
	}
	msg.Src = g.toDriver.AsRemote()
	msg.Dst = g.toDriverRemote.AsRemote()

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.finishedKernelsCount--

	return true
}

func (g *GPUController) dispatchThreadblocksToSMs() bool {
	if len(g.freeSMs) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}

	sm := g.freeSMs[0]
	threadblock := g.undispatchedThreadblocks[0]

	msg := &message.DeviceToSMMsg{
		Threadblock: *threadblock,
	}
	msg.Src = g.toSMs.AsRemote()
	msg.Dst = sm.GetPortByName(fmt.Sprintf("%s.ToGPU", sm.Name())).AsRemote()

	err := g.toSMs.Send(msg)
	if err != nil {
		return false
	}

	g.freeSMs = g.freeSMs[1:]
	g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]

	return false
}

func (g *GPUController) LogStatus() {
}

func uint32ToBytes(data uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, data)

	return bytes
}
