package component

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
)

const threadblockStackSize = 4

type GPU struct {
	*sim.TickingComponent

	tickCount int64

	// meta
	toDriverSrc sim.Port
	toDriverDst sim.Port

	subcoreCount int64
	subcores     []*SubCoreInfo
	freeSubcores []int64

	threadblocksCount            int64
	threadblocks                 []*ThreadblockInfo
	finishedThreadblocksToReport []int64
	needMoreThreadblocks         int64
}

type ThreadblockInfo struct {
	threadblock Threadblock
	finished    bool

	nextWarpToRun     int64
	finishedWarpCount int64
}

type SubCoreInfo struct {
	device *Subcore

	toSubcoreSrc sim.Port
	toSubcoreDst sim.Port

	threadblockID int64
}

func NewGPU(
	name string,
	engine sim.Engine,
	freq sim.Freq,
	subcoreCount int64,
) *GPU {
	g := &GPU{
		subcoreCount: subcoreCount,
		subcores:     make([]*SubCoreInfo, subcoreCount),
	}

	g.TickingComponent = sim.NewTickingComponent(name, engine, freq, g)
	g.toDriverSrc = sim.NewLimitNumMsgPort(g, 4, "ToDriver")

	for i := int64(0); i < g.subcoreCount; i++ {
		p := sim.NewLimitNumMsgPort(g, 4, "ToSubcore")
		subcore := NewSubcore(fmt.Sprintf("Subcore(%d)", i), engine, freq, p)

		subcoreInfo := &SubCoreInfo{
			device:       subcore,
			toSubcoreSrc: p,
			toSubcoreDst: subcore.toGPUSrc,
		}

		g.subcores[i] = subcoreInfo
		g.freeSubcores = append(g.freeSubcores, i)
	}

	return g
}

func (g *GPU) Tick(now sim.VTimeInSec) bool {
	if g.tickCount%1000 == 0 {
		fmt.Printf("    Subcore free: %d/%d\n", len(g.freeSubcores), g.subcoreCount)
	}
	g.tickCount++

	madeProgress := false

	madeProgress = g.reportFinishedThreadblocks(now) || madeProgress
	madeProgress = g.requestMoreThreadblocks(now) || madeProgress
	madeProgress = g.applyWarpToSubcores(now) || madeProgress
	madeProgress = g.processUpInput(now) || madeProgress
	madeProgress = g.processDownInput(now) || madeProgress

	return madeProgress
}

// DeviceToSubcoreMsg: apply a warp to a subcore
type DeviceToSubcoreMsg struct {
	sim.MsgMeta

	warp Warp
}

func (m *DeviceToSubcoreMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// SubcoreToDeviceMsg: report a finished warp
type SubcoreToDeviceMsg struct {
	sim.MsgMeta

	subcoreID int64
}

func (m *SubcoreToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (g *GPU) processUpInput(now sim.VTimeInSec) bool {
	msg := g.toDriverSrc.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *DriverToDeviceMsg:
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
		case *SubcoreToDeviceMsg:
			g.processSubcoreMsg(msg, now, i)
			return true
		default:
			panic("Unhandled message type")
		}
	}

	return false
}

func (g *GPU) processDriverMsg(msg *DriverToDeviceMsg, now sim.VTimeInSec) {
	if msg.newKernel {
		g.threadblocksCount = 0
		g.toDriverSrc.Retrieve(now)
		return
	}

	var threadblockID int64

	if g.threadblocksCount < threadblockStackSize {
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
		threadblock: msg.threadblock,
		finished:    false,

		nextWarpToRun:     0,
		finishedWarpCount: 0,
	}

	g.threadblocks[threadblockID] = threadblockInfo

	g.toDriverSrc.Retrieve(now)
}

func (g *GPU) processSubcoreMsg(msg *SubcoreToDeviceMsg, now sim.VTimeInSec, subcoreID int64) {
	threadblockID := g.subcores[subcoreID].threadblockID
	threadblock := g.threadblocks[threadblockID]
	threadblock.finishedWarpCount++
	g.freeSubcores = append(g.freeSubcores, msg.subcoreID)

	if threadblock.finishedWarpCount == threadblock.threadblock.WarpsCount() {
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

	msg := &DeviceToDriverMsg{
		threadBlockFinished: true,
	}
	msg.Src = g.toDriverSrc
	msg.Dst = g.toDriverDst
	msg.SendTime = now

	err := g.toDriverSrc.Send(msg)
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

	msg := &DeviceToDriverMsg{
		requestMore: true,
	}
	msg.Src = g.toDriverSrc
	msg.Dst = g.toDriverDst
	msg.SendTime = now

	err := g.toDriverSrc.Send(msg)
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
		warp := *threadblock.threadblock.Warp(threadblock.nextWarpToRun)

		msg := &DeviceToSubcoreMsg{
			warp: warp,
		}
		msg.Src = subcore.toSubcoreSrc
		msg.Dst = subcore.toSubcoreDst
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
