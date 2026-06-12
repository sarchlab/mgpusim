package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/tracing"
)

// dispatchMW retrieves requests from the Top port into the pending queue and
// dispatches pending requests to banks, skipping blocked banks to avoid
// head-of-line blocking.
type dispatchMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *dispatchMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

// Tick advances dispatch. While paused, nothing moves. While draining,
// already-accepted pending requests still dispatch to banks (so a Drain can
// converge), but no new traffic is taken from the Top port.
func (m *dispatchMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := m.dispatchPending()

	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		madeProgress = m.drainTopPort() || madeProgress
	}

	return madeProgress
}

// drainTopPort reads all incoming messages into the pending queue.
func (m *dispatchMW) drainTopPort() bool {
	madeProgress := false
	next := &m.comp.State

	for {
		msgI := m.topPort().PeekIncoming()
		if msgI == nil {
			break
		}

		if _, ok := msgI.(memprotocol.AccessReq); !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msgI)
		}

		m.topPort().RetrieveIncoming()
		tracing.TraceReqReceive(m.comp, msgI)
		next.PendingReqs = append(next.PendingReqs, msgToItem(msgI))
		madeProgress = true
	}

	return madeProgress
}

// dispatchPending tries to dispatch pending requests to banks, skipping
// blocked banks to avoid head-of-line blocking.
func (m *dispatchMW) dispatchPending() bool {
	madeProgress := false
	spec := m.comp.Spec()
	next := &m.comp.State
	remaining := make([]bankPipelineItemState, 0, len(next.PendingReqs))

	for _, item := range next.PendingReqs {
		addr := bankSelectionAddress(spec, itemAddress(item))

		bankID := selectBank(spec, addr)
		if bankID < 0 || bankID >= spec.NumBanks {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}
		b := &next.Banks[bankID]

		if rowBufferEnabled(spec) {
			rowAddr := rowAddress(spec, addr)

			if b.RowValid && b.LastRowAddr == rowAddr {
				// Row hit — accept into the pipeline directly.
				if !pipelineCanAccept(*b, spec) {
					remaining = append(remaining, item)
					continue
				}
				pipelineAccept(b, spec, item)
			} else {
				// Row miss — wait out the penalty in the delay queue.
				b.DelayQueue = append(b.DelayQueue, delayedItemState{
					Item:       item,
					CyclesLeft: spec.RowMissDelay,
				})
			}

			b.LastRowAddr = rowAddr
			b.RowValid = true
		} else {
			// No row-buffer tracking.
			if !pipelineCanAccept(*b, spec) {
				remaining = append(remaining, item)
				continue
			}
			pipelineAccept(b, spec, item)
		}

		madeProgress = true
	}

	next.PendingReqs = remaining

	return madeProgress
}
