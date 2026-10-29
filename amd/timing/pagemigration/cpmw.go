package pagemigration

import (
	"fmt"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Port names of the page-migration middleware of a command processor.
const (
	// CPDriverPort connects to the driver (the inter-device network).
	CPDriverPort = "PageMigration"

	// CPCtrlPort connects to the Control ports of the address translators
	// and the TLBs and to the Top port of the data mover of the GPU.
	CPCtrlPort = "PageMigrationCtrl"
)

// Kinds of the operations of a command processor.
const (
	cpOpHold      = "hold"
	cpOpShootdown = "shootdown"
	cpOpRelease   = "release"
	cpOpCopy      = "copy"
)

// cpOpState is a request of the driver that the command processor is
// serving. Step is the step of a shootdown (pause, invalidate, enable), and
// PendingAcks the acknowledgments that the current step waits for.
type cpOpState struct {
	ID          uint64               `json:"id"`
	Src         messaging.RemotePort `json:"src"`
	Kind        string               `json:"kind"`
	PID         vm.PID               `json:"pid"`
	VAddrs      []uint64             `json:"vaddrs"`
	SrcPAddr    uint64               `json:"src_paddr"`
	DstPAddr    uint64               `json:"dst_paddr"`
	Size        uint64               `json:"size"`
	Started     bool                 `json:"started"`
	Step        int                  `json:"step"`
	PendingAcks int                  `json:"pending_acks"`
}

// CPState is the page-migration state of a command processor. The
// destinations are set by the platform builder.
type CPState struct {
	AddressTranslators []messaging.RemotePort `json:"address_translators"`
	TLBs               []messaging.RemotePort `json:"tlbs"`
	DataMover          messaging.RemotePort   `json:"data_mover"`

	// Ops are the requests being served, in arrival order. Only one
	// shootdown runs at a time, since its steps pause and enable every TLB.
	Ops []cpOpState `json:"ops"`

	// Pending maps the ID of each control or data-move request in flight to
	// the ID of its operation.
	Pending map[uint64]uint64 `json:"pending"`

	// Messages waiting to be sent.
	CtrlOut   []memcontrolprotocol.Req            `json:"ctrl_out"`
	MoveOut   []datamoverprotocol.DataMoveRequest `json:"move_out"`
	DriverOut []Rsp                               `json:"driver_out"`
}

// NewCPState returns an empty CPState.
func NewCPState() CPState {
	return CPState{Pending: make(map[uint64]uint64)}
}

// Host is the component that a page-migration middleware runs in.
type Host interface {
	Name() string
	GetPortByName(name string) messaging.Port
	CurrentTime() timing.VTimeInPicoSec
	TickLater()
}

// CPMiddleware is the page-migration middleware of the command processor
// of a GPU. It serves the HoldReq, ShootdownReq, ReleaseReq, and CopyReq of
// the driver by sending control verbs to the address translators and the
// TLBs and data-move requests to the data mover of the GPU, and answers
// once they are acknowledged. All of its state is in CPState.
type CPMiddleware struct {
	host  Host
	state *CPState
}

// NewCPMiddleware returns the middleware of a command processor that keeps
// its state in state.
func NewCPMiddleware(host Host, state *CPState) *CPMiddleware {
	return &CPMiddleware{host: host, state: state}
}

func (m *CPMiddleware) port(name string) messaging.Port {
	return m.host.GetPortByName(name)
}

// Tick sends what is ready, takes in the acknowledgments, and accepts new
// requests from the driver.
func (m *CPMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.send() || madeProgress
	madeProgress = m.recvAcks() || madeProgress
	madeProgress = m.recvDriver() || madeProgress

	return madeProgress
}

func (m *CPMiddleware) send() bool {
	s := m.state
	madeProgress := false

	ctrl := m.port(CPCtrlPort)
	for len(s.CtrlOut) > 0 && ctrl.CanSend() {
		ctrl.Send(s.CtrlOut[0])
		s.CtrlOut = s.CtrlOut[1:]
		madeProgress = true
	}

	for len(s.MoveOut) > 0 && ctrl.CanSend() {
		ctrl.Send(s.MoveOut[0])
		s.MoveOut = s.MoveOut[1:]
		madeProgress = true
	}

	driver := m.port(CPDriverPort)
	for len(s.DriverOut) > 0 && driver.CanSend() {
		driver.Send(s.DriverOut[0])
		s.DriverOut = s.DriverOut[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *CPMiddleware) recvDriver() bool {
	port := m.port(CPDriverPort)
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true
		meta := msg.Meta()
		op := cpOpState{ID: meta.ID, Src: meta.Src}

		switch req := msg.(type) {
		case HoldReq:
			op.Kind, op.PID, op.VAddrs = cpOpHold, req.PID, req.VAddrs
		case ShootdownReq:
			op.Kind, op.PID, op.VAddrs = cpOpShootdown, req.PID, req.VAddrs
		case ReleaseReq:
			op.Kind, op.PID, op.VAddrs = cpOpRelease, req.PID, req.VAddrs
		case CopyReq:
			op.Kind = cpOpCopy
			op.SrcPAddr, op.DstPAddr, op.Size = req.SrcPAddr, req.DstPAddr,
				req.Size
		default:
			log.Panicf("page-migration CP middleware cannot handle %s",
				reflect.TypeOf(msg))
		}

		m.state.Ops = append(m.state.Ops, op)
		m.startOps()
	}
}

// startOps starts the operations that may start: every hold, release, and
// copy, and the oldest shootdown if no shootdown runs.
func (m *CPMiddleware) startOps() {
	shootdownRunning := false

	for i := range m.state.Ops {
		op := &m.state.Ops[i]

		if op.Kind == cpOpShootdown {
			if op.Started {
				shootdownRunning = true
				continue
			}

			if shootdownRunning {
				continue
			}

			shootdownRunning = true
		}

		if !op.Started {
			op.Started = true
			m.startStep(op)
		}
	}
}

// startStep sends the requests of the current step of an operation.
func (m *CPMiddleware) startStep(op *cpOpState) {
	switch op.Kind {
	case cpOpHold:
		m.sendCtrl(op, m.state.AddressTranslators, memcontrolprotocol.CmdHold)
	case cpOpRelease:
		m.sendCtrl(op, m.state.AddressTranslators,
			memcontrolprotocol.CmdRelease)
	case cpOpShootdown:
		cmds := []memcontrolprotocol.Command{
			memcontrolprotocol.CmdPause,
			memcontrolprotocol.CmdInvalidate,
			memcontrolprotocol.CmdEnable,
		}
		m.sendCtrl(op, m.state.TLBs, cmds[op.Step])
	case cpOpCopy:
		m.sendCopy(op)
	}
}

func (m *CPMiddleware) sendCtrl(
	op *cpOpState,
	dsts []messaging.RemotePort,
	cmd memcontrolprotocol.Command,
) {
	if len(dsts) == 0 {
		panic("page migration: the command processor knows no " +
			"address translator or TLB")
	}

	for _, dst := range dsts {
		req := memcontrolprotocol.Req{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          m.port(CPCtrlPort).AsRemote(),
				Dst:          dst,
				TrafficBytes: ctrlMsgBytes,
				TrafficClass: "memcontrolprotocol.Req",
			},
			Command: cmd,
		}

		if cmd != memcontrolprotocol.CmdPause &&
			cmd != memcontrolprotocol.CmdEnable {
			req.PID = op.PID
			req.Addresses = op.VAddrs
		}

		m.state.CtrlOut = append(m.state.CtrlOut, req)
		m.state.Pending[req.ID] = op.ID
		op.PendingAcks++
	}
}

func (m *CPMiddleware) sendCopy(op *cpOpState) {
	req := datamoverprotocol.DataMoveRequest{
		MsgMeta: messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          m.port(CPCtrlPort).AsRemote(),
			Dst:          m.state.DataMover,
			TrafficBytes: ctrlMsgBytes,
			TrafficClass: "datamoverprotocol.DataMoveRequest",
		},
		SrcAddress: op.SrcPAddr,
		DstAddress: op.DstPAddr,
		ByteSize:   op.Size,
		SrcSide:    "outside",
		DstSide:    "inside",
	}

	m.state.MoveOut = append(m.state.MoveOut, req)
	m.state.Pending[req.ID] = op.ID
	op.PendingAcks++
}

func (m *CPMiddleware) recvAcks() bool {
	port := m.port(CPCtrlPort)
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		switch rsp := msg.(type) {
		case memcontrolprotocol.Rsp:
			if !rsp.Success {
				panic(fmt.Sprintf("page migration: control command %d to %s "+
					"failed: %s", rsp.Command, rsp.Src, rsp.Error))
			}
		case datamoverprotocol.DataMoveResponse:
		default:
			log.Panicf("page-migration CP middleware cannot handle %s",
				reflect.TypeOf(msg))
		}

		m.ackReceived(msg.Meta().RspTo)
	}
}

func (m *CPMiddleware) ackReceived(reqID uint64) {
	opID, ok := m.state.Pending[reqID]
	if !ok {
		panic(fmt.Sprintf("page migration: unknown acknowledgment of %d",
			reqID))
	}

	delete(m.state.Pending, reqID)

	op := m.findOp(opID)
	op.PendingAcks--

	if op.PendingAcks == 0 {
		m.stepDone(opID)
	}
}

// stepDone advances an operation whose step has been acknowledged, and
// answers the driver when the operation is done.
func (m *CPMiddleware) stepDone(opID uint64) {
	op := m.findOp(opID)

	if op.Kind == cpOpShootdown && op.Step < 2 {
		op.Step++
		m.startStep(op)

		return
	}

	m.state.DriverOut = append(m.state.DriverOut, Rsp{
		MsgMeta: messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          m.port(CPDriverPort).AsRemote(),
			Dst:          op.Src,
			RspTo:        op.ID,
			TrafficBytes: ctrlMsgBytes,
			TrafficClass: "pagemigration.Rsp",
		},
	})

	for i := range m.state.Ops {
		if m.state.Ops[i].ID == opID {
			m.state.Ops = append(m.state.Ops[:i], m.state.Ops[i+1:]...)
			break
		}
	}

	m.startOps()
}

func (m *CPMiddleware) findOp(id uint64) *cpOpState {
	for i := range m.state.Ops {
		if m.state.Ops[i].ID == id {
			return &m.state.Ops[i]
		}
	}

	panic(fmt.Sprintf("page migration: unknown operation %d", id))
}
