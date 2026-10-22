package pagemigration

import (
	"fmt"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// engineMW implements the engine. tlbIndex is derived from the Spec.
type engineMW struct {
	comp     *Engine
	tlbIndex map[messaging.RemotePort]int
}

func (m *engineMW) port(name string) messaging.Port {
	return m.comp.GetPortByName(name)
}

// Tick sends what is ready, then takes in what has arrived.
func (m *engineMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendRemote() || madeProgress
	madeProgress = m.sendPulls() || madeProgress
	madeProgress = m.sendMem() || madeProgress
	madeProgress = m.sendCtrl() || madeProgress
	madeProgress = m.recvRemote() || madeProgress
	madeProgress = m.recvL2() || madeProgress
	madeProgress = m.recvDRAM() || madeProgress
	madeProgress = m.recvCtrl() || madeProgress

	return madeProgress
}

// sendRemote sends the responses to pulls, copies, and invalidations.
func (m *engineMW) sendRemote() bool {
	state := &m.comp.State
	port := m.port("Remote")
	madeProgress := false

	for len(state.PullRsps) > 0 && port.CanSend() {
		rsp := state.PullRsps[0]
		port.Send(rsp)
		state.ServedBytes += uint64(len(rsp.Data))
		state.PullRsps = state.PullRsps[1:]
		madeProgress = true
	}

	for len(state.CopyRsps) > 0 && port.CanSend() {
		port.Send(state.CopyRsps[0])
		state.CopyRsps = state.CopyRsps[1:]
		madeProgress = true
	}

	for len(state.InvalidateRsps) > 0 && port.CanSend() {
		port.Send(state.InvalidateRsps[0])
		state.InvalidateRsps = state.InvalidateRsps[1:]
		madeProgress = true
	}

	return madeProgress
}

// sendPulls starts pulling queued copies while the pull window allows.
func (m *engineMW) sendPulls() bool {
	state := &m.comp.State
	port := m.port("Remote")
	madeProgress := false

	for len(state.Copies) > 0 && port.CanSend() &&
		len(state.Pulling)+len(state.Landing) < m.comp.Spec().PullWindow {
		c := state.Copies[0]
		req := PullReq{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          port.AsRemote(),
				Dst:          c.Source,
				TrafficBytes: pullReqBytes,
				TrafficClass: "pagemigration.PullReq",
			},
			PAddr: c.SrcPAddr,
			Size:  c.Size,
		}
		port.Send(req)

		state.Pulling[req.ID] = c
		state.Copies = state.Copies[1:]
		madeProgress = true
	}

	return madeProgress
}

// sendMem issues the DRAM writes and the L2 reads within the memory window.
func (m *engineMW) sendMem() bool {
	state := &m.comp.State
	window := m.comp.Spec().MemWindow
	madeProgress := false

	dramPort := m.port("DRAM")
	for len(state.Writes) > 0 && state.DRAMInflight < window &&
		dramPort.CanSend() {
		dramPort.Send(state.Writes[0])
		state.Writes = state.Writes[1:]
		state.DRAMInflight++
		madeProgress = true
	}

	l2Port := m.port("L2")
	for len(state.Reads) > 0 && state.L2Inflight < window && l2Port.CanSend() {
		l2Port.Send(state.Reads[0])
		state.Reads = state.Reads[1:]
		state.L2Inflight++
		madeProgress = true
	}

	return madeProgress
}

func (m *engineMW) sendCtrl() bool {
	state := &m.comp.State
	port := m.port("TLBControl")
	madeProgress := false

	for len(state.CtrlReqs) > 0 && port.CanSend() {
		port.Send(state.CtrlReqs[0])
		state.CtrlReqs = state.CtrlReqs[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *engineMW) recvRemote() bool {
	port := m.port("Remote")
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		switch msg := msg.(type) {
		case CopyReq:
			m.acceptCopy(msg)
		case PullReq:
			m.serve(msg)
		case PullRsp:
			m.land(msg)
		case InvalidateReq:
			m.acceptInvalidation(msg)
		default:
			log.Panicf("page-migration engine cannot handle %s",
				reflect.TypeOf(msg))
		}
	}
}

func (m *engineMW) acceptCopy(req CopyReq) {
	state := &m.comp.State
	state.Copies = append(state.Copies, copyState{
		ReqID:     req.ID,
		Requester: req.Src,
		Source:    req.Source,
		SrcPAddr:  req.SrcPAddr,
		DstPAddr:  req.DstPAddr,
		Size:      req.Size,
	})
}

// serve splits a pull into line reads to the local L2.
func (m *engineMW) serve(req PullReq) {
	state := &m.comp.State
	line := m.comp.Spec().LineBytes
	l2Port := m.port("L2")

	state.Serving[req.ID] = servingState{
		Requester: req.Src,
		Data:      make([]byte, req.Size),
		LinesLeft: int(req.Size / line),
	}

	for offset := uint64(0); offset < req.Size; offset += line {
		addr := req.PAddr + offset
		read := memprotocol.ReadReq{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          l2Port.AsRemote(),
				Dst:          m.comp.Resources().L2Mapper.Find(addr),
				TrafficBytes: 12,
				TrafficClass: "memprotocol.ReadReq",
			},
			Address:        addr,
			AccessByteSize: line,
		}
		state.Reads = append(state.Reads, read)
		state.ReadToPull[read.ID] = lineReadState{PullID: req.ID, Offset: offset}
	}
}

// land splits the data of a pull into line writes to the local DRAM.
func (m *engineMW) land(rsp PullRsp) {
	state := &m.comp.State

	c, ok := state.Pulling[rsp.RspTo]
	if !ok {
		panic(fmt.Sprintf("%s: unknown pull %d", m.comp.Name(), rsp.RspTo))
	}

	delete(state.Pulling, rsp.RspTo)
	state.PulledBytes += uint64(len(rsp.Data))

	line := m.comp.Spec().LineBytes
	dramPort := m.port("DRAM")
	c.LinesLeft = int(c.Size / line)

	for offset := uint64(0); offset < c.Size; offset += line {
		addr := c.DstPAddr + offset
		write := memprotocol.WriteReq{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          dramPort.AsRemote(),
				Dst:          m.comp.Resources().DRAMMapper.Find(addr),
				TrafficBytes: int(line) + 12,
				TrafficClass: "memprotocol.WriteReq",
			},
			Address: addr,
			Data:    rsp.Data[offset : offset+line],
		}
		state.Writes = append(state.Writes, write)
		state.WriteToPull[write.ID] = rsp.RspTo
	}

	state.Landing[rsp.RspTo] = c
}

func (m *engineMW) recvL2() bool {
	state := &m.comp.State
	port := m.port("L2")
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		rsp, ok := msg.(memprotocol.DataReadyRsp)
		if !ok {
			log.Panicf("page-migration engine cannot handle %s from the L2",
				reflect.TypeOf(msg))
		}

		state.L2Inflight--

		lr, ok := state.ReadToPull[rsp.RspTo]
		if !ok {
			panic(fmt.Sprintf("%s: unknown read %d", m.comp.Name(), rsp.RspTo))
		}

		delete(state.ReadToPull, rsp.RspTo)

		s := state.Serving[lr.PullID]
		copy(s.Data[lr.Offset:], rsp.Data)
		s.LinesLeft--

		if s.LinesLeft > 0 {
			state.Serving[lr.PullID] = s
			continue
		}

		delete(state.Serving, lr.PullID)
		state.PullRsps = append(state.PullRsps, PullRsp{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          m.port("Remote").AsRemote(),
				Dst:          s.Requester,
				RspTo:        lr.PullID,
				TrafficBytes: len(s.Data) + pullRspOverhead,
				TrafficClass: "pagemigration.PullRsp",
			},
			Data: s.Data,
		})
	}
}

func (m *engineMW) recvDRAM() bool {
	state := &m.comp.State
	port := m.port("DRAM")
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		rsp, ok := msg.(memprotocol.WriteDoneRsp)
		if !ok {
			log.Panicf("page-migration engine cannot handle %s from the DRAM",
				reflect.TypeOf(msg))
		}

		state.DRAMInflight--

		pullID, ok := state.WriteToPull[rsp.RspTo]
		if !ok {
			panic(fmt.Sprintf("%s: unknown write %d", m.comp.Name(), rsp.RspTo))
		}

		delete(state.WriteToPull, rsp.RspTo)

		c := state.Landing[pullID]
		c.LinesLeft--

		if c.LinesLeft > 0 {
			state.Landing[pullID] = c
			continue
		}

		delete(state.Landing, pullID)
		state.CopyRsps = append(state.CopyRsps, CopyRsp{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          m.port("Remote").AsRemote(),
				Dst:          c.Requester,
				RspTo:        c.ReqID,
				TrafficBytes: ctrlMsgBytes,
				TrafficClass: "pagemigration.CopyRsp",
			},
		})
	}
}

// acceptInvalidation queues an invalidation and starts it if no other one
// is in progress. Invalidations run one at a time, so the verbs of two
// invalidations never interleave at a TLB.
func (m *engineMW) acceptInvalidation(req InvalidateReq) {
	state := &m.comp.State
	state.Invalidations = append(state.Invalidations, req)

	if len(state.Invalidations) == 1 {
		m.startInvalidation()
	}
}

// startInvalidation pauses every TLB for the first queued invalidation. Each
// TLB then advances on its own acknowledgements, so a TLB is paused only for
// the round trips of its own Invalidate and Enable.
func (m *engineMW) startInvalidation() {
	state := &m.comp.State
	tlbs := m.comp.Spec().TLBs

	if len(state.TLBSteps) != len(tlbs) {
		state.TLBSteps = make([]int, len(tlbs))
	}

	state.TLBsLeft = len(tlbs)
	for i, tlb := range tlbs {
		state.TLBSteps[i] = tlbPausing
		m.queueCtrl(tlb, memcontrolprotocol.CmdPause)
	}

	if len(tlbs) == 0 {
		m.finishInvalidation()
	}
}

func (m *engineMW) queueCtrl(
	dst messaging.RemotePort,
	cmd memcontrolprotocol.Command,
) {
	state := &m.comp.State
	req := memcontrolprotocol.Req{
		MsgMeta: messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          m.port("TLBControl").AsRemote(),
			Dst:          dst,
			TrafficClass: "memcontrolprotocol.Req",
		},
		Command: cmd,
	}

	if cmd == memcontrolprotocol.CmdInvalidate {
		inv := state.Invalidations[0]
		req.Addresses = inv.VAddrs
		req.PID = inv.PID
	}

	state.CtrlReqs = append(state.CtrlReqs, req)
}

func (m *engineMW) recvCtrl() bool {
	state := &m.comp.State
	port := m.port("TLBControl")
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		rsp, ok := msg.(memcontrolprotocol.Rsp)
		if !ok {
			log.Panicf("page-migration engine cannot handle %s from a TLB",
				reflect.TypeOf(msg))
		}

		if !rsp.Success {
			panic(fmt.Sprintf("%s: control command %d to %s failed: %s",
				m.comp.Name(), rsp.Command, rsp.Src, rsp.Error))
		}

		i := m.tlbIndexOf(rsp.Src)
		tlb := m.comp.Spec().TLBs[i]

		switch state.TLBSteps[i] {
		case tlbPausing:
			state.TLBSteps[i] = tlbInvalidating
			m.queueCtrl(tlb, memcontrolprotocol.CmdInvalidate)
		case tlbInvalidating:
			state.TLBSteps[i] = tlbEnabling
			m.queueCtrl(tlb, memcontrolprotocol.CmdEnable)
		case tlbEnabling:
			state.TLBSteps[i] = tlbIdle
			state.TLBsLeft--

			if state.TLBsLeft == 0 {
				m.finishInvalidation()
			}
		default:
			panic(fmt.Sprintf("%s: unexpected response from %s",
				m.comp.Name(), rsp.Src))
		}
	}
}

func (m *engineMW) tlbIndexOf(port messaging.RemotePort) int {
	if m.tlbIndex == nil {
		m.tlbIndex = make(map[messaging.RemotePort]int)
		for i, tlb := range m.comp.Spec().TLBs {
			m.tlbIndex[tlb] = i
		}
	}

	i, ok := m.tlbIndex[port]
	if !ok {
		panic(fmt.Sprintf("%s: %s is not a TLB of the GPU", m.comp.Name(), port))
	}

	return i
}

func (m *engineMW) finishInvalidation() {
	state := &m.comp.State
	inv := state.Invalidations[0]
	state.Invalidations = state.Invalidations[1:]

	state.InvalidateRsps = append(state.InvalidateRsps, InvalidateRsp{
		MsgMeta: messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          m.port("Remote").AsRemote(),
			Dst:          inv.Src,
			RspTo:        inv.ID,
			TrafficBytes: ctrlMsgBytes,
			TrafficClass: "pagemigration.InvalidateRsp",
		},
	})

	if len(state.Invalidations) > 0 {
		m.startInvalidation()
	}
}
