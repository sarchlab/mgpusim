package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type tickFinalizeMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *tickFinalizeMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *tickFinalizeMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	madeProgress := m.finalizeBanks()
	madeProgress = m.tickPipelines() || madeProgress
	return madeProgress
}

func (m *tickFinalizeMW) finalizeBanks() bool {
	madeProgress := false
	state := &m.comp.State

	for i := range state.Banks {
		for {
			progress := m.finalizeSingle(&state.Banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *tickFinalizeMW) finalizeSingle(b *bankState) bool {
	item, ok := bufferPeek(*b)
	if !ok {
		return false
	}

	if item.IsRead {
		return m.finalizeRead(b, &item)
	}

	return m.finalizeWrite(b, &item)
}

func (m *tickFinalizeMW) finalizeRead(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	readReq := &item.ReadMsg

	if !item.Committed {
		data, err := m.comp.Resources().Storage.Read(
			readReq.Address, readReq.AccessByteSize)
		if err != nil {
			log.Panic(err)
		}

		item.ReadData = data
		item.Committed = true

		// Update the buffer head with the committed state.
		b.PostPipelineBuf.UpdateFront(*item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	rsp := memprotocol.DataReadyRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = readReq.Src
	rsp.RspTo = readReq.ID
	rsp.Data = item.ReadData
	rsp.TrafficBytes = len(item.ReadData) + 4
	rsp.TrafficClass = "memprotocol.DataReadyRsp"

	m.topPort().Send(rsp)

	tracing.TraceReqComplete(m.comp, &item.ReadMsg)

	bufferPop(b)

	return true
}

func (m *tickFinalizeMW) finalizeWrite(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	writeReq := &item.WriteMsg

	if !item.Committed {
		addr := writeReq.Address

		if writeReq.DirtyMask == nil {
			if err := m.comp.Resources().Storage.Write(addr, writeReq.Data); err != nil {
				log.Panic(err)
			}
		} else {
			data, err := m.comp.Resources().Storage.Read(addr, uint64(len(writeReq.Data)))
			if err != nil {
				log.Panic(err)
			}

			for i := range writeReq.Data {
				if writeReq.DirtyMask[i] {
					data[i] = writeReq.Data[i]
				}
			}

			if err := m.comp.Resources().Storage.Write(addr, data); err != nil {
				log.Panic(err)
			}
		}

		item.Committed = true
		b.PostPipelineBuf.UpdateFront(*item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	rsp := memprotocol.WriteDoneRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = writeReq.Src
	rsp.RspTo = writeReq.ID
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "memprotocol.WriteDoneRsp"

	m.topPort().Send(rsp)

	tracing.TraceReqComplete(m.comp, &item.WriteMsg)

	bufferPop(b)

	return true
}

func (m *tickFinalizeMW) tickPipelines() bool {
	madeProgress := false
	state := &m.comp.State

	for i := range state.Banks {
		madeProgress = pipelineTick(&state.Banks[i]) || madeProgress
	}

	return madeProgress
}
