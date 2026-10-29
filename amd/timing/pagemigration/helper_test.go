package pagemigration

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

type testComp = modeling.Component[modeling.None, modeling.None, modeling.None]

// newTestHost builds a component with the given ports, each plugged into a
// connection that delivers nothing, so that the test drives the ports.
func newTestHost(name string, ports ...string) *testComp {
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	comp := modeling.NewBuilder[modeling.None, modeling.None, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		Build(name)

	for _, p := range ports {
		comp.DeclarePort(p)
		port := modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(comp).
			WithSpec(modeling.PortSpec{BufSize: 1024}).
			Build(p)
		comp.AssignPort(p, port)
		(&noopConn{}).PlugIn(port)
	}

	return comp
}

// drain returns every message the port has sent.
func drain(port messaging.Port) []messaging.Msg {
	var msgs []messaging.Msg

	for {
		msg := port.RetrieveOutgoing()
		if msg == nil {
			return msgs
		}

		msgs = append(msgs, msg)
	}
}

func rspTo(msg messaging.Msg) Rsp {
	return Rsp{MsgMeta: messaging.MsgMeta{
		ID:    timing.GetIDGenerator().Generate(),
		Src:   msg.Meta().Dst,
		Dst:   msg.Meta().Src,
		RspTo: msg.Meta().ID,
	}}
}
