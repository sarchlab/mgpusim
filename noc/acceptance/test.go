package acceptance

import (
	"log"
	"math/rand"

	"github.com/sarchlab/akita/v3/sim"
)

type trafficMsg struct {
	sim.MsgMeta
}

func (m *trafficMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Test is a test case.
type Test struct {
	agents            []*Agent
	msgs              []sim.Msg
	receivedMsgs      []sim.Msg
	receivedMsgsTable map[sim.Msg]bool
}

// NewTest creates a new test.
func NewTest() *Test {
	t := &Test{}
	t.receivedMsgsTable = make(map[sim.Msg]bool)
	return t
}

// RegisterAgent adds an agent to the Test
func (t *Test) RegisterAgent(agent *Agent) {
	t.agents = append(t.agents, agent)
}

// GenerateMsgs generates n message from a random source port to a random
// destination port.
func (t *Test) GenerateMsgs(n uint64) {
	for i := uint64(0); i < n; i++ {
		srcAgentID := rand.Intn(len(t.agents))
		srcAgent := t.agents[srcAgentID]
		srcPortID := rand.Intn(len(srcAgent.AgentPorts))
		srcPort := srcAgent.AgentPorts[srcPortID]

		dstAgentID := rand.Intn(len(t.agents))
		for dstAgentID == srcAgentID {
			dstAgentID = rand.Intn(len(t.agents))
		}

		dstAgent := t.agents[dstAgentID]
		dstPortID := rand.Intn(len(dstAgent.AgentPorts))
		dstPort := dstAgent.AgentPorts[dstPortID]

		msg := &trafficMsg{}
		msg.Meta().ID = sim.GetIDGenerator().Generate()
		msg.Src = srcPort
		msg.Dst = dstPort
		msg.TrafficBytes = rand.Intn(4096)
		// msg.TrafficBytes = 512
		srcAgent.MsgsToSend = append(srcAgent.MsgsToSend, msg)
		t.registerMsg(msg)
	}
}

func (t *Test) registerMsg(msg sim.Msg) {
	t.msgs = append(t.msgs, msg)
}

// receiveMsg marks that a message is received.
func (t *Test) receiveMsg(msg sim.Msg, recvPort sim.Port) {
	t.msgMustBeReceivedAtItsDestination(msg, recvPort)
	t.msgMustNotBeReceivedBefore(msg)
	t.receivedMsgs = append(t.receivedMsgs, msg)
	// log.Printf("Msg %s: sent at %.10f, recved at %.10f",
	// 	msg.Meta().ID, msg.Meta().SendTime, msg.Meta().RecvTime)
}

func (t *Test) msgMustBeReceivedAtItsDestination(
	msg sim.Msg,
	recvPort sim.Port,
) {
	if msg.Meta().Dst != recvPort {
		panic("msg delivered to a wrong destination")
	}
}

func (t *Test) msgMustNotBeReceivedBefore(msg sim.Msg) {
	if _, found := t.receivedMsgsTable[msg]; found {
		panic("msg is double delivered")
	}
	t.receivedMsgsTable[msg] = true
}

// MustHaveReceivedAllMsgs asserts that all the messages sent are received.
func (t *Test) MustHaveReceivedAllMsgs() {
	if len(t.msgs) == len(t.receivedMsgs) {
		return
	}

	for _, sentMsg := range t.msgs {
		if _, found := t.receivedMsgsTable[sentMsg]; !found {
			log.Printf("msg %s expected, but not received\n", sentMsg.Meta().ID)
		}
	}

	panic("some messages are dropped")
}

// ReportBandwidthAchieved dumps the bandwidth observed by each agents.
func (t *Test) ReportBandwidthAchieved(now sim.VTimeInSec) {
	for _, a := range t.agents {
		log.Printf("agent %s, send bandwidth %.2f GB/s, recv bandwidth %.2f GB/s",
			a.Name(),
			float64(a.sendBytes)/float64(now)/1e9,
			float64(a.recvBytes)/float64(now)/1e9)
	}
}
