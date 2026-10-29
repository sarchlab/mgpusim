package rdma

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// countRemoteAccess counts a request that the engine forwarded from the
// local L1 caches to a remote GPU.
func (m *forwardMiddleware) countRemoteAccess(req memprotocol.AccessReq) {
	spec := m.comp.Spec()
	if !spec.CountAccesses {
		return
	}

	counter := &m.comp.State.AccessCounter
	addr := req.GetAddress()

	counter.RemoteAccesses++

	if _, ok := req.(memprotocol.ReadReq); ok {
		counter.RemoteReads++

		if spec.DeviceMemSize > 0 {
			counter.RemoteReadsByOwner[addr/spec.DeviceMemSize]++
		}
	}

	if !spec.NotifiesAccesses() {
		return
	}

	page := addr >> spec.CounterLog2PageSize
	counter.Counts[page]++

	if counter.Counts[page] < spec.CounterThreshold {
		return
	}

	counter.Notifications = append(counter.Notifications,
		AccessCounterNotification{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Dst:          spec.CounterNotifyDst,
				TrafficBytes: 16,
				TrafficClass: "rdma.AccessCounterNotification",
			},
			PAddr:    page << spec.CounterLog2PageSize,
			DeviceID: spec.DeviceID,
			Count:    counter.Counts[page],
		})
	delete(counter.Counts, page)
}

// sendNotifications sends the access-counter notifications.
func (m *forwardMiddleware) sendNotifications() bool {
	counter := &m.comp.State.AccessCounter
	if len(counter.Notifications) == 0 {
		return false
	}

	port := m.port("AccessCounter")
	madeProgress := false

	for len(counter.Notifications) > 0 && port.CanSend() {
		n := counter.Notifications[0]
		n.Src = port.AsRemote()
		port.Send(n)

		counter.Notifications = counter.Notifications[1:]
		counter.NumNotifications++
		madeProgress = true
	}

	return madeProgress
}
