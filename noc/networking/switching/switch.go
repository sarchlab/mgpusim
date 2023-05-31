package switching

import (
	"fmt"

	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
	"github.com/sarchlab/mgpusim/v3/noc/networking/arbitration"
	"github.com/sarchlab/mgpusim/v3/noc/networking/routing"
)

type flitPipelineItem struct {
	taskID string
	flit   *messaging.Flit
}

func (f flitPipelineItem) TaskID() string {
	return f.taskID
}

// A portComplex is the infrastructure related to a port.
type portComplex struct {
	// localPort is the port that is equipped on the switch.
	localPort sim.Port

	// remotePort is the port that is connected to the localPort.
	remotePort sim.Port

	// Data arrived at the local port needs to be processed in a pipeline. There
	// is a processing pipeline for each local port.
	pipeline pipelining.Pipeline

	// The flits here are buffered after the pipeline and are waiting to be
	// assigned with an output buffer.
	routeBuffer sim.Buffer

	// The flits here are buffered to wait to be forwarded to the output buffer.
	forwardBuffer sim.Buffer

	// The flits here are waiting to be sent to the next hop.
	sendOutBuffer sim.Buffer

	// NumInputChannel is the number of flits that can stream into the
	// switch from the port. The RouteBuffer and the ForwardBuffer should
	// have the capacity of this number.
	numInputChannel int

	// NumOutputChannel is the number of flits that can stream out of the
	// switch to the port. The SendOutBuffer should have the capacity of this
	// number.
	numOutputChannel int
}

// Switch is an Akita component that can forward request to destination.
type Switch struct {
	*sim.TickingComponent

	ports                []sim.Port
	portToComplexMapping map[sim.Port]portComplex
	routingTable         routing.Table
	arbiter              arbitration.Arbiter
}

// addPort adds a new port on the switch.
func (s *Switch) addPort(complex portComplex) {
	s.ports = append(s.ports, complex.localPort)
	s.portToComplexMapping[complex.localPort] = complex
	s.arbiter.AddBuffer(complex.forwardBuffer)
}

// GetRoutingTable returns the routine table used by the switch.
func (s *Switch) GetRoutingTable() routing.Table {
	return s.routingTable
}

// Tick update the Switch's state.
func (s *Switch) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.sendOut(now) || madeProgress
	madeProgress = s.forward(now) || madeProgress
	madeProgress = s.route(now) || madeProgress
	madeProgress = s.movePipeline(now) || madeProgress
	madeProgress = s.startProcessing(now) || madeProgress

	return madeProgress
}

func (s *Switch) flitParentTaskID(flit *messaging.Flit) string {
	return flit.ID + "_e2e"
}

func (s *Switch) flitTaskID(flit *messaging.Flit) string {
	return flit.ID + "_" + s.Name()
}

func (s *Switch) startProcessing(now sim.VTimeInSec) (madeProgress bool) {
	for _, port := range s.ports {
		complex := s.portToComplexMapping[port]

		for i := 0; i < complex.numInputChannel; i++ {
			item := port.Peek()
			if item == nil {
				break
			}

			if !complex.pipeline.CanAccept() {
				break
			}

			flit := item.(*messaging.Flit)
			pipelineItem := flitPipelineItem{
				taskID: s.flitTaskID(flit),
				flit:   flit,
			}
			complex.pipeline.Accept(now, pipelineItem)
			port.Retrieve(now)
			madeProgress = true

			tracing.StartTask(
				s.flitTaskID(flit),
				s.flitParentTaskID(flit),
				s, "flit", "flit_inside_sw",
				flit,
			)

			// fmt.Printf("%.10f, %s, switch recv flit, %s\n",
			// 	now, s.Name(), flit.ID)
		}
	}

	return madeProgress
}

func (s *Switch) movePipeline(now sim.VTimeInSec) (madeProgress bool) {
	for _, port := range s.ports {
		complex := s.portToComplexMapping[port]
		madeProgress = complex.pipeline.Tick(now) || madeProgress
	}

	return madeProgress
}

func (s *Switch) route(now sim.VTimeInSec) (madeProgress bool) {
	for _, port := range s.ports {
		complex := s.portToComplexMapping[port]
		routeBuf := complex.routeBuffer
		forwardBuf := complex.forwardBuffer

		for i := 0; i < complex.numInputChannel; i++ {
			item := routeBuf.Peek()
			if item == nil {
				break
			}

			if !forwardBuf.CanPush() {
				break
			}

			pipelineItem := item.(flitPipelineItem)
			flit := pipelineItem.flit
			s.assignFlitOutputBuf(flit)
			routeBuf.Pop()
			forwardBuf.Push(flit)
			madeProgress = true

			// fmt.Printf("%.10f, %s, switch route flit, %s\n",
			// 	now, s.Name(), flit.ID)
		}
	}

	return madeProgress
}

func (s *Switch) forward(now sim.VTimeInSec) (madeProgress bool) {
	inputBuffers := s.arbiter.Arbitrate(now)

	for _, buf := range inputBuffers {
		for {
			item := buf.Peek()
			if item == nil {
				break
			}

			flit := item.(*messaging.Flit)
			if !flit.OutputBuf.CanPush() {
				break
			}

			flit.OutputBuf.Push(flit)
			buf.Pop()
			madeProgress = true

			// fmt.Printf("%.10f, %s, switch forward flit, %s\n",
			// now, s.Name(), item.(*messaging.Flit).ID)
		}
	}

	return madeProgress
}

func (s *Switch) sendOut(now sim.VTimeInSec) (madeProgress bool) {
	for _, port := range s.ports {
		complex := s.portToComplexMapping[port]
		sendOutBuf := complex.sendOutBuffer

		for i := 0; i < complex.numOutputChannel; i++ {
			item := sendOutBuf.Peek()
			if item == nil {
				break
			}

			flit := item.(*messaging.Flit)
			flit.Meta().Src = complex.localPort
			flit.Meta().Dst = complex.remotePort
			flit.Meta().SendTime = now

			err := complex.localPort.Send(flit)
			if err == nil {
				sendOutBuf.Pop()
				madeProgress = true

				// fmt.Printf("%.10f, %s, switch send flit out, %s\n",
				// now, s.Name(), flit.ID)

				tracing.EndTask(s.flitTaskID(flit), s)
			}
		}
	}

	return madeProgress
}

func (s *Switch) assignFlitOutputBuf(f *messaging.Flit) {
	outPort := s.routingTable.FindPort(f.Msg.Meta().Dst)
	if outPort == nil {
		panic(fmt.Sprintf("%s: no output port for %s",
			s.Name(), f.Msg.Meta().Dst))
	}

	complex := s.portToComplexMapping[outPort]

	f.OutputBuf = complex.sendOutBuffer
	if f.OutputBuf == nil {
		panic(fmt.Sprintf("%s: no output buffer for %s",
			s.Name(), f.Msg.Meta().Dst))
	}
}

func (s *Switch) setFlitNextHopDst(f *messaging.Flit) {
	f.Src = f.Dst
	f.Dst = s.portToComplexMapping[f.Src].remotePort
}

// SwitchBuilder can build switches
type SwitchBuilder struct {
	engine       sim.Engine
	freq         sim.Freq
	routingTable routing.Table
	arbiter      arbitration.Arbiter
}

// WithEngine sets the engine that the switch to build uses.
func (b SwitchBuilder) WithEngine(engine sim.Engine) SwitchBuilder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the switch to build works at.
func (b SwitchBuilder) WithFreq(freq sim.Freq) SwitchBuilder {
	b.freq = freq
	return b
}

// WithArbiter sets the arbiter to be used by the switch to build.
func (b SwitchBuilder) WithArbiter(arbiter arbitration.Arbiter) SwitchBuilder {
	b.arbiter = arbiter
	return b
}

// WithRoutingTable sets the routing table to be used by the switch to build.
func (b SwitchBuilder) WithRoutingTable(rt routing.Table) SwitchBuilder {
	b.routingTable = rt
	return b
}

// Build creates a new switch
func (b SwitchBuilder) Build(name string) *Switch {
	b.engineMustBeGiven()
	b.freqMustNotBeZero()
	b.routingTableMustBeGiven()
	b.arbiterMustBeGiven()

	s := &Switch{}
	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.routingTable = b.routingTable
	s.arbiter = b.arbiter
	s.portToComplexMapping = make(map[sim.Port]portComplex)
	return s
}

func (b SwitchBuilder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine of switch is not given")
	}
}

func (b SwitchBuilder) freqMustNotBeZero() {
	if b.freq == 0 {
		panic("switch frequency cannot be 0")
	}
}

func (b SwitchBuilder) routingTableMustBeGiven() {
	if b.routingTable == nil {
		panic("switch requires a routing table to operate")
	}
}

func (b SwitchBuilder) arbiterMustBeGiven() {
	if b.arbiter == nil {
		panic("switch requires an arbiter to operate")
	}
}

// SwitchPortAdder can add a port to a switch.
type SwitchPortAdder struct {
	sw               *Switch
	localPort        sim.Port
	remotePort       sim.Port
	latency          int
	numInputChannel  int
	numOutputChannel int
}

// MakeSwitchPortAdder creates a SwitchPortAdder that can add ports for the
// provided switch.
func MakeSwitchPortAdder(sw *Switch) SwitchPortAdder {
	return SwitchPortAdder{
		sw:               sw,
		numInputChannel:  1,
		numOutputChannel: 1,
		latency:          1,
	}
}

// WithPorts defines the ports to add. The local port is part of the switch.
// The remote port is the port on an endpoint or on another switch.
func (a SwitchPortAdder) WithPorts(local, remote sim.Port) SwitchPortAdder {
	a.localPort = local
	a.remotePort = remote
	return a
}

// WithLatency sets the latency of the port.
func (a SwitchPortAdder) WithLatency(latency int) SwitchPortAdder {
	a.latency = latency
	return a
}

// WithNumInputChannel sets the number of input channels of the port. This
// number determines the number of flits that can be injected into the switch
// from the port in each cycle.
func (a SwitchPortAdder) WithNumInputChannel(num int) SwitchPortAdder {
	a.numInputChannel = num
	return a
}

// WithNumOutputChannel sets the number of output channels of the port. This
// number determines the number of flits that can be ejected from the switch
// to the port in each cycle.
func (a SwitchPortAdder) WithNumOutputChannel(num int) SwitchPortAdder {
	a.numOutputChannel = num
	return a
}

// AddPort adds the port to the switch.
func (a SwitchPortAdder) AddPort() {
	complexID := len(a.sw.ports)
	complexName := fmt.Sprintf("%s.PortComplex%d", a.sw.Name(), complexID)

	sendOutBuf := sim.NewBuffer(complexName+"SendOutBuf", a.numOutputChannel)
	forwardBuf := sim.NewBuffer(complexName+"ForwardBuf", a.numInputChannel)
	routeBuf := sim.NewBuffer(complexName+"RouteBuf", a.numInputChannel)
	pipeline := pipelining.MakeBuilder().
		WithNumStage(a.latency).
		WithCyclePerStage(1).
		WithPipelineWidth(a.numInputChannel).
		WithPostPipelineBuffer(routeBuf).
		Build(a.localPort.Name() + ".Pipeline")

	pc := portComplex{
		localPort:        a.localPort,
		remotePort:       a.remotePort,
		pipeline:         pipeline,
		routeBuffer:      routeBuf,
		forwardBuffer:    forwardBuf,
		sendOutBuffer:    sendOutBuf,
		numInputChannel:  a.numInputChannel,
		numOutputChannel: a.numOutputChannel,
	}

	a.sw.addPort(pc)
}
