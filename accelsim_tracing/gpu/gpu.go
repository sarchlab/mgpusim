package gpu

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/accelsimtracing/subcore"
	"github.com/sarchlab/akita/v3/sim"
)

type GPU struct {
	*sim.TickingComponent

	tickCount int64

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSubcores             sim.Port
	connectionWithSubcores sim.Connection

	subcoreCount int64
	subcores     []*SubCoreInfo
	freeSubcores []int64

	threadblocksCount            int64
	threadblocks                 []*ThreadblockInfo
	finishedThreadblocksToReport []int64
	needMoreThreadblocks         int64
}

type ThreadblockInfo struct {
	threadblock benchmark.Threadblock
	finished    bool

	nextWarpToRun     int64
	finishedWarpCount int64
}

type SubCoreInfo struct {
	device *subcore.Subcore

	toSubcoreRemote sim.Port

	threadblockID int64
}

func (g *GPU) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = g.reportFinishedThreadblocks(now) || madeProgress
	madeProgress = g.requestMoreThreadblocks(now) || madeProgress
	madeProgress = g.applyWarpToSubcores(now) || madeProgress
	madeProgress = g.processUpInput(now) || madeProgress
	madeProgress = g.processDownInput(now) || madeProgress

	return madeProgress
}

func (g *GPU) processUpInput(now sim.VTimeInSec) bool {
	msg := g.toDriver.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DriverToDeviceMsg:
		g.processDriverMsg(msg, now)
	default:
		panic("Unhandled message type")
	}

	return true
}

func (g *GPU) processDownInput(now sim.VTimeInSec) bool {
	for i := int64(0); i < g.subcoreCount; i++ {
		subcore := g.subcores[i]
		msg := subcore.toSubcoreSrc.Peek()
		if msg == nil {
			continue
		}

		switch msg := msg.(type) {
		case *message.SubcoreToDeviceMsg:
			g.processSubcoreMsg(msg, now, i)
			return true
		default:
			panic("Unhandled message type")
		}
	}

	return false
}

const stackSize = 4

func (g *GPU) processDriverMsg(msg *message.DriverToDeviceMsg, now sim.VTimeInSec) {
	if msg.NewKernel {
		g.threadblocksCount = 0
		g.toDriver.Retrieve(now)
		return
	}

	var threadblockID int64

	if g.threadblocksCount < stackSize {
		threadblockID = g.threadblocksCount
		g.threadblocksCount++

	} else {
		for i := int64(0); i < g.threadblocksCount; i++ {
			if g.threadblocks[i].finished {
				threadblockID = i
				break
			}
		}
	}

	threadblockInfo := &ThreadblockInfo{
		threadblock: msg.Threadblock,
		finished:    false,

		nextWarpToRun:     0,
		finishedWarpCount: 0,
	}

	g.threadblocks[threadblockID] = threadblockInfo

	g.toDriver.Retrieve(now)
}

func (g *GPU) processSubcoreMsg(msg *message.SubcoreToDeviceMsg, now sim.VTimeInSec, subcoreID int64) {
	threadblockID := g.subcores[subcoreID].threadblockID
	threadblock := g.threadblocks[threadblockID]
	threadblock.finishedWarpCount++
	g.freeSubcores = append(g.freeSubcores, msg.SubcoreID)

	if threadblock.finishedWarpCount == threadblock.threadblock.WarpsCount {
		threadblock.finished = true
		g.finishedThreadblocksToReport = append(g.finishedThreadblocksToReport, threadblockID)
		g.needMoreThreadblocks++
	}

	g.subcores[subcoreID].toSubcoreSrc.Retrieve(now)
}

func (g *GPU) reportFinishedThreadblocks(now sim.VTimeInSec) bool {
	if len(g.finishedThreadblocksToReport) == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		ThreadblockFinished: true,
	}
	msg.Src = g.toDriver
	msg.Dst = g.toDriverRemote
	msg.SendTime = now

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.finishedThreadblocksToReport = g.finishedThreadblocksToReport[1:]

	return true
}

func (g *GPU) requestMoreThreadblocks(now sim.VTimeInSec) bool {
	if g.needMoreThreadblocks == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		RequestMore: true,
	}
	msg.Src = g.toDriver
	msg.Dst = g.toDriverRemote
	msg.SendTime = now

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.needMoreThreadblocks--
	return true
}

func (g *GPU) applyWarpToSubcores(now sim.VTimeInSec) bool {
	if len(g.freeSubcores) == 0 {
		return false
	}

	subcoreID := g.freeSubcores[0]
	subcore := g.subcores[subcoreID]

	for i := int64(0); i < g.threadblocksCount; i++ {
		threadblock := g.threadblocks[i]
		if threadblock.finished {
			continue
		}
		warp := threadblock.threadblock.Warps[threadblock.nextWarpToRun]

		msg := &message.DeviceToSubcoreMsg{
			Warp: warp,
		}
		msg.Src = subcore.toSubcoreSrc
		msg.Dst = subcore.toSubcoreRemote
		msg.SendTime = now

		err := subcore.toSubcoreSrc.Send(msg)
		if err != nil {
			continue
		}

		threadblock.nextWarpToRun++
		g.freeSubcores = g.freeSubcores[1:]

		return true
	}

	return false
}
