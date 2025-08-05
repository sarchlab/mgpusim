package cp

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
)

type ctrlMiddleware struct {
	cp *CommandProcessor
}

func (m *ctrlMiddleware) Tick() {
	msg := m.cp.ToDriver.PeekIncoming()
	if msg == nil {
		return
	}

	if m.CanHandle(msg) {
		m.Handle(msg)
	}
}

func (m *ctrlMiddleware) CanHandle(msg sim.Msg) bool {
	switch msg.(type) {
	case *protocol.RDMADrainCmdFromDriver,
		*protocol.RDMARestartCmdFromDriver,
		*protocol.ShootDownCommand,
		*protocol.GPURestartReq,
		*protocol.PageMigrationReqToCP:
		return true
	default:
		return false
	}
}

func (m *ctrlMiddleware) Handle(msg sim.Msg) bool {
	switch req := msg.(type) {
	case *protocol.RDMADrainCmdFromDriver:
		return m.cp.processRDMADrainCmd(req)
	case *protocol.RDMARestartCmdFromDriver:
		return m.cp.processRDMARestartCommand(req)
	case *protocol.ShootDownCommand:
		return m.cp.processShootdownCommand(req)
	case *protocol.GPURestartReq:
		return m.cp.processGPURestartReq(req)
	case *protocol.PageMigrationReqToCP:
		return m.cp.processPageMigrationReq(req)
	}
	panic("Unhandled message")
}
