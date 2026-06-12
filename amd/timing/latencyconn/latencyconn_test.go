package latencyconn

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
)

type sampleMsg struct {
	sim.MsgMeta
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *sampleMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

type testAgent struct {
	*sim.TickingComponent

	msgsOut []sim.Msg
	msgsIn  []sim.Msg

	OutPort sim.Port
}

func newTestAgent(
	engine sim.Engine,
	freq sim.Freq,
	name string,
) *testAgent {
	a := new(testAgent)
	a.TickingComponent = sim.NewTickingComponent(name, engine, freq, a)
	a.OutPort = sim.NewPort(a, 4, 4, name+".OutPort")

	return a
}

func (a *testAgent) Tick() bool {
	madeProgress := false

	msgIn := a.OutPort.RetrieveIncoming()
	if msgIn != nil {
		a.msgsIn = append(a.msgsIn, msgIn)
		madeProgress = true
	}

	if len(a.msgsOut) > 0 {
		err := a.OutPort.Send(a.msgsOut[0])
		if err == nil {
			madeProgress = true
			a.msgsOut = a.msgsOut[1:]
		}
	}

	return madeProgress
}

func TestLatencyConnectionDeliversAllMessages(t *testing.T) {
	engine := sim.NewSerialEngine()

	conn := MakeBuilder().
		WithEngine(engine).
		WithFreq(1).
		WithLatency(5).
		Build("LatConn")

	agent1 := newTestAgent(engine, 1, "Agent1")
	agent2 := newTestAgent(engine, 1, "Agent2")

	conn.PlugIn(agent1.OutPort)
	conn.PlugIn(agent2.OutPort)

	numMsgs := 10
	for i := 0; i < numMsgs; i++ {
		msg := &sampleMsg{}
		msg.Src = agent1.OutPort.AsRemote()
		msg.Dst = agent2.OutPort.AsRemote()
		msg.ID = sim.GetIDGenerator().Generate()
		agent1.msgsOut = append(agent1.msgsOut, msg)
	}

	agent1.TickLater()
	engine.Run()

	if len(agent2.msgsIn) != numMsgs {
		t.Errorf("expected %d messages, got %d",
			numMsgs, len(agent2.msgsIn))
	}
}

func TestLatencyConnectionBuffersMessages(t *testing.T) {
	engine := sim.NewSerialEngine()
	latency := 50

	conn := MakeBuilder().
		WithEngine(engine).
		WithFreq(1).
		WithLatency(latency).
		Build("LatConn")

	agent1 := newTestAgent(engine, 1, "Agent1")
	agent2 := newTestAgent(engine, 1, "Agent2")

	conn.PlugIn(agent1.OutPort)
	conn.PlugIn(agent2.OutPort)

	msg := &sampleMsg{}
	msg.Src = agent1.OutPort.AsRemote()
	msg.Dst = agent2.OutPort.AsRemote()
	msg.ID = sim.GetIDGenerator().Generate()
	agent1.msgsOut = append(agent1.msgsOut, msg)

	agent1.TickLater()
	engine.Run()

	if len(agent2.msgsIn) != 1 {
		t.Fatalf("expected 1 message, got %d", len(agent2.msgsIn))
	}

	// With latency=50 at freq=1, the end time should be greater than
	// a zero-latency connection. The message takes at least 50 ticks
	// to traverse the connection.
	endTime := engine.CurrentTime()
	if endTime < sim.VTimeInSec(latency) {
		t.Errorf("expected end time >= %d, got %d",
			latency, endTime)
	}
}

func TestLatencyConnectionCrossGroupPenaltySameGroup(t *testing.T) {
	baseLatency := 5
	crossPenalty := 20

	engine := sim.NewSerialEngine()
	conn := MakeBuilder().
		WithEngine(engine).
		WithFreq(1).
		WithLatency(baseLatency).
		WithCrossGroupPenalty(crossPenalty).
		Build("ConnSame")

	a1 := newTestAgent(engine, 1, "A1")
	a2 := newTestAgent(engine, 1, "A2")

	conn.PlugIn(a1.OutPort)
	conn.PlugIn(a2.OutPort)

	conn.SetPortXCDGroup(a1.OutPort, 0)
	conn.SetPortXCDGroup(a2.OutPort, 0) // same group

	msg := &sampleMsg{}
	msg.Src = a1.OutPort.AsRemote()
	msg.Dst = a2.OutPort.AsRemote()
	msg.ID = sim.GetIDGenerator().Generate()
	a1.msgsOut = append(a1.msgsOut, msg)

	a1.TickLater()
	engine.Run()

	if len(a2.msgsIn) != 1 {
		t.Fatalf("expected 1 message, got %d", len(a2.msgsIn))
	}

	sameGroupTime := engine.CurrentTime()

	if sameGroupTime < sim.VTimeInSec(baseLatency) {
		t.Errorf("same-group time (%v) should be >= %d",
			sameGroupTime, baseLatency)
	}
}

func TestLatencyConnectionCrossGroupPenaltyCrossGroup(t *testing.T) {
	baseLatency := 5
	crossPenalty := 20

	engine := sim.NewSerialEngine()
	conn := MakeBuilder().
		WithEngine(engine).
		WithFreq(1).
		WithLatency(baseLatency).
		WithCrossGroupPenalty(crossPenalty).
		Build("ConnCross")

	b1 := newTestAgent(engine, 1, "B1")
	b3 := newTestAgent(engine, 1, "B3")

	conn.PlugIn(b1.OutPort)
	conn.PlugIn(b3.OutPort)

	conn.SetPortXCDGroup(b1.OutPort, 0)
	conn.SetPortXCDGroup(b3.OutPort, 1) // different group

	msg := &sampleMsg{}
	msg.Src = b1.OutPort.AsRemote()
	msg.Dst = b3.OutPort.AsRemote()
	msg.ID = sim.GetIDGenerator().Generate()
	b1.msgsOut = append(b1.msgsOut, msg)

	b1.TickLater()
	engine.Run()

	if len(b3.msgsIn) != 1 {
		t.Fatalf("expected 1 message, got %d", len(b3.msgsIn))
	}

	crossGroupTime := engine.CurrentTime()

	sameGroupBaseline := sim.VTimeInSec(baseLatency)
	if crossGroupTime <= sameGroupBaseline {
		t.Errorf("cross-group time (%v) should be > base latency (%v)",
			crossGroupTime, sameGroupBaseline)
	}

	expectedCrossMin := sim.VTimeInSec(baseLatency + crossPenalty)
	if crossGroupTime < expectedCrossMin {
		t.Errorf("cross-group time (%v) should be >= %v",
			crossGroupTime, expectedCrossMin)
	}
}

func TestLatencyConnectionBidirectional(t *testing.T) {
	engine := sim.NewSerialEngine()

	conn := MakeBuilder().
		WithEngine(engine).
		WithFreq(1).
		WithLatency(3).
		Build("LatConn")

	agent1 := newTestAgent(engine, 1, "Agent1")
	agent2 := newTestAgent(engine, 1, "Agent2")

	conn.PlugIn(agent1.OutPort)
	conn.PlugIn(agent2.OutPort)

	msg1 := &sampleMsg{}
	msg1.Src = agent1.OutPort.AsRemote()
	msg1.Dst = agent2.OutPort.AsRemote()
	msg1.ID = sim.GetIDGenerator().Generate()
	agent1.msgsOut = append(agent1.msgsOut, msg1)

	msg2 := &sampleMsg{}
	msg2.Src = agent2.OutPort.AsRemote()
	msg2.Dst = agent1.OutPort.AsRemote()
	msg2.ID = sim.GetIDGenerator().Generate()
	agent2.msgsOut = append(agent2.msgsOut, msg2)

	agent1.TickLater()
	agent2.TickLater()
	engine.Run()

	if len(agent1.msgsIn) != 1 {
		t.Errorf("agent1 expected 1 message, got %d", len(agent1.msgsIn))
	}

	if len(agent2.msgsIn) != 1 {
		t.Errorf("agent2 expected 1 message, got %d", len(agent2.msgsIn))
	}
}
