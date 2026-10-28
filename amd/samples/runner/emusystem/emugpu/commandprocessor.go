package emugpu

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// The emulation platform does not use amd/timing/cp (the timing command
// processor models dispatch latency, caches, and control flows that have no
// meaning in functional emulation). Instead, this package provides a minimal
// command processor: it decomposes a LaunchKernelReq into per-work-group
// MapWGReqs, dispatches them to the emulation ComputeUnit, and replies to the
// driver with a LaunchKernelRsp once every work-group of the kernel is
// acknowledged. FlushReqs are acknowledged immediately (there are no caches).
//
// Memory copies are handled directly by the driver's magic-memory-copy
// middleware (the emulation platform shares one global mem.Storage between the
// driver and the ComputeUnit), so the command processor does not need a DMA
// engine.

const (
	cpDriverPortName = "ToDriver"
	cpCUPortName     = "ToCU"
)

// Spec is the immutable configuration of the emulation command processor.
type Spec struct {
	Freq  timing.Freq          `json:"freq"`
	CUDst messaging.RemotePort `json:"cu_dst"`
}

// DefaultSpec returns the emulation command processor's default frequency.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

// cpState contains queued work and all in-flight launch bookkeeping.
type cpState struct {
	WGsToSend, RspsToSend []messaging.Msg
	Launches              map[uint64]kernelLaunch
	WGToKernel            map[uint64]uint64
}

// Comp is the emulation command processor.
type Comp = ticking.Component[Spec, cpState, modeling.None, cpPorts, cpMiddlewares]
type cpPorts struct {
	ToDriver messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
	ToCU     messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
}
type cpMiddlewares struct{ CP *cpMiddleware }

func (m *cpMiddlewares) Initialize(c *Comp) { m.CP = &cpMiddleware{comp: c} }

type kernelLaunch struct {
	ReqID       uint64
	ReqSrc      messaging.RemotePort
	Outstanding int
}
type cpMiddleware struct{ comp *Comp }

func (s *cpState) Initialize(*Comp) {
	s.Launches = make(map[uint64]kernelLaunch)
	s.WGToKernel = make(map[uint64]uint64)
}

func buildCommandProcessor(reg modeling.Simulation, freq timing.Freq, dst messaging.RemotePort, name string) *Comp {
	spec := DefaultSpec()
	spec.Freq, spec.CUDst = freq, dst
	return modeling.MakeBuilder[*Comp](spec).WithSimulation(reg).Build(name)
}

func (m *cpMiddleware) driverPort() messaging.Port {
	return m.comp.Ports.ToDriver
}

func (m *cpMiddleware) cuPort() messaging.Port {
	return m.comp.Ports.ToCU
}

// Tick advances the command processor by one cycle.
func (m *cpMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendMapWGReqs() || madeProgress
	madeProgress = m.sendLaunchRsps() || madeProgress
	madeProgress = m.processCUInput() || madeProgress
	madeProgress = m.processDriverInput() || madeProgress

	return madeProgress
}

func (m *cpMiddleware) sendMapWGReqs() bool {
	madeProgress := false
	port := m.cuPort()

	for len(m.comp.State.WGsToSend) > 0 && port.CanSend() {
		port.Send(m.comp.State.WGsToSend[0])
		m.comp.State.WGsToSend = m.comp.State.WGsToSend[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *cpMiddleware) sendLaunchRsps() bool {
	madeProgress := false
	port := m.driverPort()

	for len(m.comp.State.RspsToSend) > 0 && port.CanSend() {
		port.Send(m.comp.State.RspsToSend[0])
		m.comp.State.RspsToSend = m.comp.State.RspsToSend[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *cpMiddleware) processDriverInput() bool {
	msg, ok := m.driverPort().PeekIncoming()
	if !ok {
		return false
	}

	m.driverPort().RetrieveIncoming()

	switch msg.Payload.(type) {
	case protocol.LaunchKernelReq:
		req := msg

		m.handleLaunchKernelReq(req)
	case protocol.FlushReq:
		req := msg

		// No caches in the emulation platform; acknowledge immediately.
		m.comp.State.RspsToSend = append(m.comp.State.RspsToSend, messaging.Msg{ID: m.comp.NewID(),
			Src:   m.driverPort().AsRemote(),
			Dst:   req.Src,
			RspTo: req.ID, Payload: protocol.GeneralRsp{}},
		)
	default:
		panic("emugpu command processor: unsupported message type")
	}

	return true
}

func (m *cpMiddleware) handleLaunchKernelReq(req messaging.Msg) {
	gridBuilder := kernels.NewGridBuilder(m.comp)
	gridBuilder.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: req.Payload.(protocol.LaunchKernelReq).CodeObject.Restore(),
		Packet:     kernels.PacketValue{Present: true, Packet: req.Payload.(protocol.LaunchKernelReq).Packet}.Restore(),
		PacketAddr: req.Payload.(protocol.LaunchKernelReq).PacketAddress,
		Selection:  req.Payload.(protocol.LaunchKernelReq).Selection,
	})

	numWG := gridBuilder.NumWG()
	if numWG == 0 {
		m.comp.State.RspsToSend = append(m.comp.State.RspsToSend, m.makeLaunchRsp(req.ID, req.Src))
		return
	}

	m.comp.State.Launches[req.ID] = kernelLaunch{
		ReqID:       req.ID,
		ReqSrc:      req.Src,
		Outstanding: numWG,
	}

	cuSrc := m.cuPort().AsRemote()
	for i := 0; i < numWG; i++ {
		wg := gridBuilder.NextWG()

		mapReq := messaging.Msg{ID: m.comp.NewID(),
			Src: cuSrc,
			Dst: m.comp.Spec.CUDst, Payload: protocol.MapWGReq{WorkGroup: kernels.WorkGroupSnapshot(wg),
				PID: req.Payload.(protocol.LaunchKernelReq).PID}}

		m.comp.State.WGToKernel[mapReq.ID] = req.ID
		m.comp.State.WGsToSend = append(m.comp.State.WGsToSend, mapReq)
	}
}

func (m *cpMiddleware) processCUInput() bool {
	msg, ok := m.cuPort().PeekIncoming()
	if !ok {
		return false
	}

	completion := msg
	m.cuPort().RetrieveIncoming()

	for _, wgID := range completion.Payload.(protocol.WGCompletionMsg).RspToIDs {
		kernelID, ok := m.comp.State.WGToKernel[wgID]
		if !ok {
			continue
		}

		delete(m.comp.State.WGToKernel, wgID)

		launch := m.comp.State.Launches[kernelID]
		launch.Outstanding--
		m.comp.State.Launches[kernelID] = launch
		if launch.Outstanding == 0 {
			m.comp.State.RspsToSend = append(m.comp.State.RspsToSend,
				m.makeLaunchRsp(launch.ReqID, launch.ReqSrc))
			delete(m.comp.State.Launches, kernelID)
		}
	}

	return true
}

func (m *cpMiddleware) makeLaunchRsp(
	reqID uint64,
	dst messaging.RemotePort,
) messaging.Msg {
	return messaging.Msg{ID: m.comp.NewID(),
		Src:   m.driverPort().AsRemote(),
		Dst:   dst,
		RspTo: reqID, Payload: protocol.LaunchKernelRsp{}}
}

func (m *cpMiddleware) Handle(timing.Event) bool { return m.Tick() }
