package gpu

import (
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/dram"
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

	// toSMMem       sim.Port
	// toSMMemRemote sim.Port

	// toDRAM       sim.Port
	// toDRAMRemote sim.Port

	// L2Caches    []*writeback.Comp
	// L2CacheSize uint64
	Drams    []*dram.Comp
	DramSize uint64

	// PendingReadReq  map[string]*mem.ReadReq
	// PendingWriteReq map[string]*mem.WriteReq

	// PendingSMtoGPUMemReadReq  map[string]*message.SMToGPUMemReadMsg
	// PendingSMtoGPUMemWriteReq map[string]*message.SMToGPUMemWriteMsg

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
	// madeProgress = g.processSMsInputMem() || madeProgress
	// madeProgress = g.processDRAMRsp() || madeProgress

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

// func (g *GPUController) processSMsInputMem() bool {
// 	msg := g.toSMMem.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}

// 	switch msg := msg.(type) {
// 	case *message.SMToGPUMemReadMsg:
// 		g.processSMMemReadMsg(msg)
// 	case *message.SMToGPUMemWriteMsg:
// 		g.processSMMemWriteMsg(msg)
// 	default:
// 		log.WithField("function", "processSMsInputMem").Panic("Unhandled message type")
// 	}

// 	return true
// }

// func (g *GPUController) processDRAMRsp() bool {
// 	msg := g.toDRAM.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}
// 	// fmt.Printf("%T\n", msg)
// 	switch msg := msg.(type) {
// 	case *mem.WriteDoneRsp:
// 		// write := g.PendingWriteReq[msg.RespondTo]
// 		fmt.Printf("%.10f, GPUController received msg from Dram, write complete\n",
// 			g.CurrentTime())

// 		g.processDRAMWriteRspMsg(msg)
// 		// delete(g.PendingWriteReq, msg.RespondTo)

// 		// g.toDRAM.RetrieveIncoming()

// 		return true
// 	case *mem.DataReadyRsp:
// 		// req := g.PendingReadReq[msg.RespondTo]
// 		// delete(g.PendingReadReq, msg.RespondTo)

// 		fmt.Printf("%.10f, GPUController received msg from Dram, read complete, %v\n",
// 			g.CurrentTime(), msg.Data)

// 		// delete(g.PendingReadReq, msg.RespondTo)
// 		g.processDRAMReadRspMsg(msg)
// 		// g.toDRAM.RetrieveIncoming()

// 		return true
// 	default:
// 		log.Panicf("cannot process message of type %s", reflect.TypeOf(msg))
// 	}

// 	return false
// }

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

// func (g *GPUController) processSMMemReadMsg(msg *message.SMToGPUMemReadMsg) bool {
// 	// fmt.Printf("%.10f, %s, GPUController, read from address = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address)
// 	readReq := mem.ReadReqBuilder{}.
// 		WithSrc(g.toDRAM.AsRemote()).
// 		WithDst(g.toDRAMRemote.AsRemote()).
// 		WithAddress(msg.Address).
// 		WithPID(1).
// 		Build()
// 	err := g.toDRAM.Send(readReq)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send read request to DRAM: %v\n", err)
// 		g.toSMMem.RetrieveIncoming()
// 		return false
// 	}
// 	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n",
// 	// 	g.Engine.CurrentTime(), msg.Address, readReq.ID)
// 	g.PendingReadReq[readReq.ID] = readReq
// 	g.PendingSMtoGPUMemReadReq[readReq.ID] = msg
// 	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, readReq.ID)
// 	g.toSMMem.RetrieveIncoming()
// 	return true
// }

// func (g *GPUController) processSMMemWriteMsg(msg *message.SMToGPUMemWriteMsg) bool {
// 	// fmt.Printf("%.10f, %s, GPUController, write to address = %d, data = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address, msg.Data)
// 	writeReq := mem.WriteReqBuilder{}.
// 		WithSrc(g.toDRAM.AsRemote()).
// 		WithDst(g.toDRAMRemote.AsRemote()).
// 		WithAddress(msg.Address).
// 		WithPID(1).
// 		WithData(uint32ToBytes(msg.Data)).
// 		Build()

// 	err := g.toDRAM.Send(writeReq)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send write request to DRAM: %v\n", err)
// 		g.toSMMem.RetrieveIncoming()
// 		return false
// 	}
// 	g.PendingWriteReq[writeReq.ID] = writeReq
// 	g.PendingSMtoGPUMemWriteReq[writeReq.ID] = msg
// 	// fmt.Printf("%.10f, GPUController, write request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, writeReq.ID)
// 	g.toSMMem.RetrieveIncoming()
// 	return true
// }

// func (g *GPUController) processDRAMReadRspMsg(rspMsg *mem.DataReadyRsp) bool {
// 	fmt.Printf("%.10f, %s, GPUController\n", g.Engine.CurrentTime(), g.Name())
// 	msg := &message.GPUtoSMMemReadMsg{
// 		Address:           g.PendingReadReq[rspMsg.RespondTo].Address,
// 		Rsp:               *rspMsg,
// 		OriginalSMtoGPUID: g.PendingSMtoGPUMemReadReq[rspMsg.RespondTo].ID,
// 	}
// 	msg.Src = g.toSMMem.AsRemote()
// 	msg.Dst = g.PendingReadReq[rspMsg.RespondTo].Src
// 	// err := g.toSMMem.Send(msg)
// 	// if err != nil {
// 	// 	fmt.Printf("GPUController failed to send read rsp to SMController: %v\n", err)
// 	// 	g.toDRAM.RetrieveIncoming()
// 	// 	return false
// 	// }
// 	g.toDRAM.RetrieveIncoming()
// 	return true
// }

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
