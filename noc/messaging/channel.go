package messaging

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type channelEnd struct {
	port            sim.Port
	srcSideBuf      sim.Buffer
	postPipelineBuf sim.Buffer
	pipeline        pipelining.Pipeline
	busy            bool
}

// Channel connects two ports and can deliver messages with configurable
// latencies.
type Channel struct {
	*sim.TickingComponent

	left, right *channelEnd
	ends        map[sim.Port]*channelEnd

	pipelineNumStages      int
	pipelineCyclesPerStage int
	pipelineWidth          int
}

// PlugIn marks the port to be connected with the connection.
func (c *Channel) PlugIn(port sim.Port, sourceSideBufSize int) {
	c.Lock()
	defer c.Unlock()

	bufSide := ""
	if c.left == nil {
		bufSide = "Left"
	} else if c.right == nil {
		bufSide = "Right"
	} else {
		panic("one channel can only connect with two ports.")
	}

	end := &channelEnd{
		port: port,
		srcSideBuf: sim.NewBuffer(
			c.Name()+"."+bufSide+"SrcBuf",
			sourceSideBufSize,
		),
		postPipelineBuf: sim.NewBuffer(
			c.Name()+"."+bufSide+"PostPipelineBuf",
			sourceSideBufSize,
		),
	}

	end.pipeline = pipelining.MakeBuilder().
		WithNumStage(c.pipelineNumStages).
		WithPipelineWidth(c.pipelineWidth).
		WithCyclePerStage(c.pipelineCyclesPerStage).
		WithPostPipelineBuffer(end.postPipelineBuf).
		Build(c.Name() + "." + port.Name() + ".Pipeline")

	if c.left == nil {
		c.left = end
	} else if c.right == nil {
		c.right = end
	} else {
		panic("one channel can only connect with two ports.")
	}

	port.SetConnection(c)
	c.ends[port] = end
}

// Unplug removes the association between the port and the channel.
func (c *Channel) Unplug(port sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Channel) NotifyAvailable(now sim.VTimeInSec, port sim.Port) {
	c.TickLater(now)
}

// CanSend returns true if the channel can send a message from a port.
func (c *Channel) CanSend(port sim.Port) bool {
	c.Lock()
	defer c.Unlock()

	canSend := c.ends[port].srcSideBuf.CanPush()

	if !canSend {
		c.ends[port].busy = true
	}

	return canSend
}

// Send of a Channel schedules a DeliveryEvent immediately
func (c *Channel) Send(msg sim.Msg) *sim.SendError {
	c.Lock()
	defer c.Unlock()

	c.msgMustBeValid(msg)

	srcEnd := c.ends[msg.Meta().Src]

	if !srcEnd.srcSideBuf.CanPush() {
		srcEnd.busy = true
		return sim.NewSendError()
	}

	srcEnd.srcSideBuf.Push(msg)

	// fmt.Printf("%.10f, %s, send, %s\n",
	// 	c.Engine.CurrentTime(), c.Name(), msg.Meta().ID)

	c.TickNow(msg.Meta().SendTime)

	return nil
}

// Tick moves the messages in the channel forward and delivers messages when
// possible.
func (c *Channel) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.deliver(now) || madeProgress
	madeProgress = c.moveMsg(now) || madeProgress

	return madeProgress
}

type msgPipeTask struct {
	msg sim.Msg
}

func (t msgPipeTask) TaskID() string {
	return t.msg.Meta().ID + ".pipeline"
}

func (c *Channel) channelMsgTaskID(msg sim.Msg) string {
	return msg.Meta().ID + "_" + c.Name()
}

func (c *Channel) moveMsg(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.moveMsgOneEnd(now, c.left) || madeProgress
	madeProgress = c.moveMsgOneEnd(now, c.right) || madeProgress

	return madeProgress
}

func (c *Channel) moveMsgOneEnd(now sim.VTimeInSec, end *channelEnd) bool {
	madeProgress := false

	madeProgress = end.pipeline.Tick(now) || madeProgress

	for end.srcSideBuf.Size() > 0 {
		if !end.pipeline.CanAccept() {
			break
		}

		msg := end.srcSideBuf.Pop().(sim.Msg)
		end.pipeline.Accept(now, msgPipeTask{msg: msg})

		if end.busy {
			end.port.NotifyAvailable(now)
			end.busy = false
		}

		madeProgress = true

		// fmt.Printf("Flit kind: [%s]\n", flitKind)
		c.traceTransferTaskStart(end, msg, now)
	}

	return madeProgress
}

func (c *Channel) traceTransferTaskStart(
	end *channelEnd,
	msg sim.Msg,
	now sim.VTimeInSec,
) {
	if c.NumHooks() == 0 {
		return
	}

	var direction string
	if end == c.left {
		direction = c.right.port.Name() + "." + c.left.port.Name()
	} else if end == c.right {
		direction = c.left.port.Name() + "." + c.right.port.Name()
	} else {
		panic("channel internal error")
	}

	flitKind := fmt.Sprintf("flit.%s", reflect.TypeOf(msg.(*Flit).Msg))

	tracing.StartTaskWithSpecificLocation(
		c.channelMsgTaskID(msg),
		// associate w/ parent msg task ID, see networking/switching/endpoint.go
		fmt.Sprintf("msg_%s_e2e", msg.Meta().ID),
		c, flitKind, "flit_through_channel", direction, msg)
}

func (c *Channel) deliver(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.deliverOneEnd(now, c.left, c.right) || madeProgress
	madeProgress = c.deliverOneEnd(now, c.right, c.left) || madeProgress

	return madeProgress
}

func (c *Channel) deliverOneEnd(
	now sim.VTimeInSec,
	srcEnd, dstEnd *channelEnd,
) bool {
	madeProgress := false

	for srcEnd.postPipelineBuf.Size() > 0 {
		msgTask := srcEnd.postPipelineBuf.Peek().(msgPipeTask)
		msg := msgTask.msg
		msg.Meta().RecvTime = now

		err := dstEnd.port.Recv(msg)
		if err != nil {
			break
		}

		// fmt.Printf("%.10f, %s, delivered, %s, %s\n",
		// c.Engine.CurrentTime(), c.Name(), msg.Meta().ID, dstEnd.port.Name())

		srcEnd.postPipelineBuf.Pop()
		madeProgress = true

		tracing.EndTask(c.channelMsgTaskID(msg), c)
	}

	return madeProgress
}

func (c *Channel) msgMustBeValid(msg sim.Msg) {
	c.portMustNotBeNil(msg.Meta().Src)
	c.portMustNotBeNil(msg.Meta().Dst)
	c.portMustBeConnected(msg.Meta().Src)
	c.portMustBeConnected(msg.Meta().Dst)
	c.srcDstMustNotBeTheSame(msg)
}

func (c *Channel) portMustNotBeNil(port sim.Port) {
	if port == nil {
		panic("src or dst is not given")
	}
}

func (c *Channel) portMustBeConnected(port sim.Port) {
	if _, connected := c.ends[port]; !connected {
		panic("src or dst is not connected")
	}
}

func (c *Channel) srcDstMustNotBeTheSame(msg sim.Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}

// ChannelBuilder can build channels.
type ChannelBuilder struct {
	engine                 sim.Engine
	freq                   sim.Freq
	pipelineNumStages      int
	pipelineCyclesPerStage int
	pipelineWidth          int
}

// MakeChannelBuilder creates a ChannelBuilder.
func MakeChannelBuilder() ChannelBuilder {
	return ChannelBuilder{
		pipelineNumStages:      1,
		pipelineCyclesPerStage: 1,
		pipelineWidth:          1,
	}
}

// WithEngine sets the engine of the channel to be built.
func (b ChannelBuilder) WithEngine(e sim.Engine) ChannelBuilder {
	b.engine = e
	return b
}

// WithFreq sets the frequency of the channel to be built.
func (b ChannelBuilder) WithFreq(f sim.Freq) ChannelBuilder {
	b.freq = f
	return b
}

// WithPipelineParameters sets the parameters of the channel internel pipeline.
func (b ChannelBuilder) WithPipelineParameters(
	numStage, cyclesPerStage, width int,
) ChannelBuilder {
	b.pipelineNumStages = numStage
	b.pipelineCyclesPerStage = cyclesPerStage
	b.pipelineWidth = width
	return b
}

// Build creates a new channel with the given name.
func (b ChannelBuilder) Build(name string) *Channel {
	c := &Channel{
		pipelineNumStages:      b.pipelineNumStages,
		pipelineCyclesPerStage: b.pipelineCyclesPerStage,
		pipelineWidth:          b.pipelineWidth,
	}

	b.engineMustBeSet()
	b.freqMustBeSet()

	c.TickingComponent = sim.NewSecondaryTickingComponent(
		name, b.engine, b.freq, c,
	)

	c.ends = make(map[sim.Port]*channelEnd)

	return c
}

func (b ChannelBuilder) engineMustBeSet() {
	if b.engine == nil {
		panic("engine is not given")
	}
}

func (b ChannelBuilder) freqMustBeSet() {
	if b.freq == 0 {
		panic("freq must be set")
	}
}
