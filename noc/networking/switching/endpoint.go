package switching

import (
	"container/list"
	"fmt"
	"math"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
)

type msgToAssemble struct {
	msg             sim.Msg
	numFlitRequired int
	numFlitArrived  int
}

// EndPoint is an akita component that delegates sending and receiving actions
// of a few ports.
type EndPoint struct {
	*sim.TickingComponent

	DevicePorts      []sim.Port
	NetworkPort      sim.Port
	DefaultSwitchDst sim.Port

	numInputChannels  int
	numOutputChannels int
	flitByteSize      int
	encodingOverhead  float64
	msgOutBuf         []sim.Msg
	msgOutBufSize     int
	flitsToSend       []*messaging.Flit

	assemblingMsgTable map[string]*list.Element
	assemblingMsgs     *list.List
	assembledMsgs      []sim.Msg
}

// CanSend returns whether the endpoint can send a message.
func (ep *EndPoint) CanSend(src sim.Port) bool {
	ep.Lock()
	defer ep.Unlock()

	return len(ep.msgOutBuf) < ep.msgOutBufSize
}

// Send initiates a message sending process. It breaks down the message into
// flits and send the flits to the external connections.
func (ep *EndPoint) Send(msg sim.Msg) *sim.SendError {
	ep.Lock()
	defer ep.Unlock()

	if len(ep.msgOutBuf) >= ep.msgOutBufSize {
		return &sim.SendError{}
	}

	ep.msgOutBuf = append(ep.msgOutBuf, msg)

	ep.TickLater(msg.Meta().SendTime)

	ep.logMsgE2ETask(msg, false)

	return nil
}

// PlugIn connects a port to the endpoint.
func (ep *EndPoint) PlugIn(port sim.Port, srcBufCap int) {
	port.SetConnection(ep)
	ep.DevicePorts = append(ep.DevicePorts, port)
	ep.msgOutBufSize = srcBufCap
}

// NotifyAvailable triggers the endpoint to continue to tick.
func (ep *EndPoint) NotifyAvailable(now sim.VTimeInSec, port sim.Port) {
	ep.TickLater(now)
}

// Unplug removes the association of a port and an endpoint.
func (ep *EndPoint) Unplug(port sim.Port) {
	panic("not implemented")
}

// Tick update the endpoint state.
func (ep *EndPoint) Tick(now sim.VTimeInSec) bool {
	ep.Lock()
	defer ep.Unlock()

	madeProgress := false

	madeProgress = ep.sendFlitOut(now) || madeProgress
	madeProgress = ep.prepareFlits(now) || madeProgress
	madeProgress = ep.tryDeliver(now) || madeProgress
	madeProgress = ep.assemble(now) || madeProgress
	madeProgress = ep.recv(now) || madeProgress

	return madeProgress
}

func (ep *EndPoint) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (ep *EndPoint) flitTaskID(flit sim.Msg) string {
	return fmt.Sprintf("%s_e2e", flit.Meta().ID)
}

func (ep *EndPoint) sendFlitOut(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < ep.numOutputChannels; i++ {
		if len(ep.flitsToSend) == 0 {
			return madeProgress
		}

		flit := ep.flitsToSend[0]
		flit.SendTime = now
		err := ep.NetworkPort.Send(flit)

		if err == nil {
			ep.flitsToSend = ep.flitsToSend[1:]

			// fmt.Printf("%.10f, %s, ep send, %s, %d\n",
			// 	ep.Engine.CurrentTime(), ep.Name(),
			// 	flit.Meta().ID, len(ep.flitsToSend))

			if len(ep.flitsToSend) == 0 {
				for _, p := range ep.DevicePorts {
					p.NotifyAvailable(now)
				}
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (ep *EndPoint) prepareFlits(now sim.VTimeInSec) bool {
	madeProgress := false

	for {
		if len(ep.msgOutBuf) == 0 {
			return madeProgress
		}

		if len(ep.msgOutBuf) > ep.numOutputChannels {
			return madeProgress
		}

		msg := ep.msgOutBuf[0]
		ep.msgOutBuf = ep.msgOutBuf[1:]
		ep.flitsToSend = append(ep.flitsToSend, ep.msgToFlits(msg)...)

		// fmt.Printf("%.10f, %s, ep send, msg-%s, %d\n",
		// 	ep.Engine.CurrentTime(), ep.Name(), msg.Meta().ID,
		// 	len(ep.flitsToSend))

		for _, flit := range ep.flitsToSend {
			ep.logFlitE2ETask(flit, false)
		}

		madeProgress = true
	}
}

func (ep *EndPoint) recv(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < ep.numInputChannels; i++ {
		received := ep.NetworkPort.Peek()
		if received == nil {
			return madeProgress
		}

		flit := received.(*messaging.Flit)
		msg := flit.Msg

		assemblingElem := ep.assemblingMsgTable[msg.Meta().ID]
		if assemblingElem == nil {
			assemblingElem = ep.assemblingMsgs.PushBack(&msgToAssemble{
				msg:             msg,
				numFlitRequired: flit.NumFlitInMsg,
				numFlitArrived:  0,
			})
			ep.assemblingMsgTable[msg.Meta().ID] = assemblingElem
		}

		assembling := assemblingElem.Value.(*msgToAssemble)
		assembling.numFlitArrived++

		ep.NetworkPort.Retrieve(now)

		ep.logFlitE2ETask(flit, true)

		madeProgress = true

		// fmt.Printf("%.10f, %s, ep received flit %s\n",
		// 	now, ep.Name(), flit.ID)
	}

	return madeProgress
}

func (ep *EndPoint) assemble(now sim.VTimeInSec) bool {
	madeProgress := false

	for e := ep.assemblingMsgs.Front(); e != nil; e = e.Next() {
		assemblingMsg := e.Value.(*msgToAssemble)

		if assemblingMsg.numFlitArrived < assemblingMsg.numFlitRequired {
			continue
		}

		ep.assembledMsgs = append(ep.assembledMsgs, assemblingMsg.msg)
		ep.assemblingMsgs.Remove(e)
		delete(ep.assemblingMsgTable, assemblingMsg.msg.Meta().ID)

		madeProgress = true
	}

	return madeProgress
}

func (ep *EndPoint) tryDeliver(now sim.VTimeInSec) bool {
	madeProgress := false

	for len(ep.assembledMsgs) > 0 {
		msg := ep.assembledMsgs[0]
		msg.Meta().RecvTime = now

		err := msg.Meta().Dst.Recv(msg)
		if err != nil {
			return madeProgress
		}

		// fmt.Printf("%.10f, %s, delivered, %s\n",
		// 	now, ep.Name(), msg.Meta().ID)
		ep.logMsgE2ETask(msg, true)

		ep.assembledMsgs = ep.assembledMsgs[1:]

		madeProgress = true
	}

	return madeProgress
}

func (ep *EndPoint) logFlitE2ETask(flit *messaging.Flit, isEnd bool) {
	if ep.NumHooks() == 0 {
		return
	}

	msg := flit.Msg

	if isEnd {
		tracing.EndTask(ep.flitTaskID(flit), ep)
		return
	}

	tracing.StartTaskWithSpecificLocation(
		ep.flitTaskID(flit), ep.msgTaskID(msg.Meta().ID),
		ep, "flit_e2e", "flit_e2e", ep.Name()+".FlitBuf", flit,
	)
}

func (ep *EndPoint) logMsgE2ETask(msg sim.Msg, isEnd bool) {
	if ep.NumHooks() == 0 {
		return
	}

	rsp, isRsp := msg.(sim.Rsp)
	if isRsp {
		ep.logMsgRsp(isEnd, rsp)
		return
	}

	ep.logMsgReq(isEnd, msg)
}

func (ep *EndPoint) logMsgReq(isEnd bool, msg sim.Msg) {
	if isEnd {
		tracing.EndTask(ep.msgTaskID(msg.Meta().ID), ep)
	} else {
		tracing.StartTask(
			ep.msgTaskID(msg.Meta().ID),
			msg.Meta().ID+"_req_out",
			ep, "msg_e2e", "msg_e2e", msg,
		)
	}
}

func (ep *EndPoint) logMsgRsp(isEnd bool, rsp sim.Rsp) {
	if isEnd {
		tracing.EndTask(ep.msgTaskID(rsp.Meta().ID), ep)
	} else {
		tracing.StartTask(
			ep.msgTaskID(rsp.Meta().ID),
			rsp.GetRspTo()+"_req_out",
			ep, "msg_e2e", "msg_e2e", rsp,
		)
	}
}

func (ep *EndPoint) msgToFlits(msg sim.Msg) []*messaging.Flit {
	numFlit := 1
	if msg.Meta().TrafficBytes > 0 {
		trafficByte := msg.Meta().TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * ep.encodingOverhead))
		numFlit = (trafficByte-1)/ep.flitByteSize + 1
	}

	flits := make([]*messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.FlitBuilder{}.
			WithSrc(ep.NetworkPort).
			WithDst(ep.DefaultSwitchDst).
			WithSeqID(i).
			WithNumFlitInMsg(numFlit).
			WithMsg(msg).
			Build()
	}

	return flits
}

// EndPointBuilder can build End Points.
type EndPointBuilder struct {
	engine                   sim.Engine
	freq                     sim.Freq
	numInputChannels         int
	numOutputChannels        int
	flitByteSize             int
	encodingOverhead         float64
	flitAssemblingBufferSize int
	networkPortBufferSize    int
	devicePorts              []sim.Port
}

// MakeEndPointBuilder creates a new EndPointBuilder with default
// configurations.
func MakeEndPointBuilder() EndPointBuilder {
	return EndPointBuilder{
		flitByteSize:             32,
		flitAssemblingBufferSize: 64,
		networkPortBufferSize:    4,
		freq:                     1 * sim.GHz,
		numInputChannels:         1,
		numOutputChannels:        1,
	}
}

// WithEngine sets the engine of the End Point to build.
func (b EndPointBuilder) WithEngine(e sim.Engine) EndPointBuilder {
	b.engine = e
	return b
}

// WithFreq sets the frequency of the End Point to built.
func (b EndPointBuilder) WithFreq(freq sim.Freq) EndPointBuilder {
	b.freq = freq
	return b
}

// WithNumInputChannels sets the number of input channels of the End Point
// to build.
func (b EndPointBuilder) WithNumInputChannels(num int) EndPointBuilder {
	b.numInputChannels = num
	return b
}

// WithNumOutputChannels sets the number of output channels of the End Point
// to build.
func (b EndPointBuilder) WithNumOutputChannels(num int) EndPointBuilder {
	b.numOutputChannels = num
	return b
}

// WithFlitByteSize sets the flit byte size that the End Point supports.
func (b EndPointBuilder) WithFlitByteSize(n int) EndPointBuilder {
	b.flitByteSize = n
	return b
}

// WithEncodingOverhead sets the encoding overhead.
func (b EndPointBuilder) WithEncodingOverhead(o float64) EndPointBuilder {
	b.encodingOverhead = o
	return b
}

// WithNetworkPortBufferSize sets the network port buffer size of the end point.
func (b EndPointBuilder) WithNetworkPortBufferSize(n int) EndPointBuilder {
	b.networkPortBufferSize = n
	return b
}

// WithDevicePorts sets a list of ports that communicate directly through the
// End Point.
func (b EndPointBuilder) WithDevicePorts(ports []sim.Port) EndPointBuilder {
	b.devicePorts = ports
	return b
}

// Build creates a new End Point.
func (b EndPointBuilder) Build(name string) *EndPoint {
	b.engineMustBeGiven()
	b.freqMustBeGiven()
	b.flitByteSizeMustBeGiven()

	ep := &EndPoint{}
	ep.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, ep)
	ep.flitByteSize = b.flitByteSize

	ep.numInputChannels = b.numInputChannels
	ep.numOutputChannels = b.numOutputChannels

	ep.assemblingMsgs = list.New()
	ep.assemblingMsgTable = make(map[string]*list.Element)

	ep.NetworkPort = sim.NewLimitNumMsgPort(
		ep, b.networkPortBufferSize,
		fmt.Sprintf("%s.NetworkPort", ep.Name()))

	for _, dp := range b.devicePorts {
		ep.PlugIn(dp, 1)
	}

	return ep
}

func (b EndPointBuilder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine is not given")
	}
}

func (b EndPointBuilder) freqMustBeGiven() {
	if b.freq == 0 {
		panic("freq must be given")
	}
}

func (b EndPointBuilder) flitByteSizeMustBeGiven() {
	if b.flitByteSize == 0 {
		panic("flit byte size must be given")
	}
}
