package cp

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
)

type cpMiddleware struct {
	cp *CommandProcessor
}

func (m *cpMiddleware) Tick() {
	msg := m.cp.ToDriver.PeekIncoming()
	if msg == nil {
		return
	}

	if m.CanHandle(msg) {
		m.Handle(msg)
	}
}

func (m *cpMiddleware) CanHandle(msg sim.Msg) bool {
	switch msg.(type) {
	case *protocol.LaunchKernelReq, *protocol.FlushReq,
		*protocol.MemCopyH2DReq, *protocol.MemCopyD2HReq:
		return true
	default:
		return false
	}
}

func (m *cpMiddleware) Handle(msg sim.Msg) bool {
	switch req := msg.(type) {
	case *protocol.LaunchKernelReq:
		return m.cp.processLaunchKernelReq(req)
	case *protocol.FlushReq:
		return m.cp.processFlushReq(req)
	case *protocol.MemCopyH2DReq, *protocol.MemCopyD2HReq:
		return m.cp.processMemCopyReq(req)
	}
	panic("unhandled message in command middleware")
}
