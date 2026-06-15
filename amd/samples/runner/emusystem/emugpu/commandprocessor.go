package emugpu

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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

// cpSpec is the immutable configuration of the emulation command processor.
type cpSpec struct {
	Freq timing.Freq `json:"freq"`
}

// cpState is the pure runtime state of the emulation command processor. The
// in-flight bookkeeping holds pointers (work-groups) and lives on the
// middleware instead.
type cpState struct{}

// commandProcessor is the emulation command processor component.
type commandProcessor = modeling.Component[cpSpec, cpState, modeling.None]

// kernelLaunch tracks one in-flight kernel launch.
type kernelLaunch struct {
	reqID       uint64
	reqSrc      messaging.RemotePort
	outstanding int
}

// cpMiddleware implements the behavior of the emulation command processor.
type cpMiddleware struct {
	comp *commandProcessor

	// cuDst is the remote port of the ComputeUnit that work-groups are
	// dispatched to. It is set during platform assembly.
	cuDst messaging.RemotePort

	// TODO(akita5): state purity — the queued requests and the in-flight
	// bookkeeping below hold pointers (work-groups) and maps and therefore
	// cannot live in the pure component State. They are not checkpointable.
	wgsToSend  []protocol.MapWGReq
	rspsToSend []messaging.Msg
	launches   map[uint64]*kernelLaunch // keyed by LaunchKernelReq.ID
	wgToKernel map[uint64]uint64        // MapWGReq.ID -> LaunchKernelReq.ID
}

func newCPMiddleware(comp *commandProcessor) *cpMiddleware {
	return &cpMiddleware{
		comp:       comp,
		launches:   make(map[uint64]*kernelLaunch),
		wgToKernel: make(map[uint64]uint64),
	}
}

// buildCommandProcessor builds the emulation command processor. It declares
// the "ToDriver" and "ToCU" ports; the port instances are assigned externally
// after Build.
func buildCommandProcessor(
	reg modeling.Registrar,
	freq timing.Freq,
	name string,
) (*commandProcessor, *cpMiddleware) {
	comp := modeling.NewBuilder[cpSpec, cpState, modeling.None]().
		WithEngine(reg.GetEngine()).
		WithFreq(freq).
		WithSpec(cpSpec{Freq: freq}).
		Build(name)
	comp.State = cpState{}

	mw := newCPMiddleware(comp)
	comp.AddMiddleware(mw)

	comp.DeclarePort(cpDriverPortName)
	comp.DeclarePort(cpCUPortName)

	reg.RegisterComponent(comp)

	return comp, mw
}

func (m *cpMiddleware) driverPort() messaging.Port {
	return m.comp.GetPortByName(cpDriverPortName)
}

func (m *cpMiddleware) cuPort() messaging.Port {
	return m.comp.GetPortByName(cpCUPortName)
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

	for len(m.wgsToSend) > 0 && port.CanSend() {
		port.Send(m.wgsToSend[0])
		m.wgsToSend = m.wgsToSend[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *cpMiddleware) sendLaunchRsps() bool {
	madeProgress := false
	port := m.driverPort()

	for len(m.rspsToSend) > 0 && port.CanSend() {
		port.Send(m.rspsToSend[0])
		m.rspsToSend = m.rspsToSend[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *cpMiddleware) processDriverInput() bool {
	msg := m.driverPort().PeekIncoming()
	if msg == nil {
		return false
	}

	m.driverPort().RetrieveIncoming()

	switch req := msg.(type) {
	case protocol.LaunchKernelReq:
		m.handleLaunchKernelReq(req)
	case protocol.FlushReq:
		// No caches in the emulation platform; acknowledge immediately.
		m.rspsToSend = append(m.rspsToSend, protocol.GeneralRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   m.driverPort().AsRemote(),
				Dst:   req.Src,
				RspTo: req.ID,
			},
		})
	default:
		panic("emugpu command processor: unsupported message type")
	}

	return true
}

func (m *cpMiddleware) handleLaunchKernelReq(req protocol.LaunchKernelReq) {
	gridBuilder := kernels.NewGridBuilder()
	gridBuilder.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: req.CodeObject,
		Packet:     req.Packet,
		PacketAddr: req.PacketAddress,
		WGFilter:   req.WGFilter,
	})

	numWG := gridBuilder.NumWG()
	if numWG == 0 {
		m.rspsToSend = append(m.rspsToSend, m.makeLaunchRsp(req.ID, req.Src))
		return
	}

	m.launches[req.ID] = &kernelLaunch{
		reqID:       req.ID,
		reqSrc:      req.Src,
		outstanding: numWG,
	}

	cuSrc := m.cuPort().AsRemote()
	for i := 0; i < numWG; i++ {
		wg := gridBuilder.NextWG()

		mapReq := protocol.MapWGReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: cuSrc,
				Dst: m.cuDst,
			},
			WorkGroup: wg,
			PID:       req.PID,
		}

		m.wgToKernel[mapReq.ID] = req.ID
		m.wgsToSend = append(m.wgsToSend, mapReq)
	}
}

func (m *cpMiddleware) processCUInput() bool {
	msg := m.cuPort().PeekIncoming()
	if msg == nil {
		return false
	}

	completion := msg.(protocol.WGCompletionMsg)
	m.cuPort().RetrieveIncoming()

	for _, wgID := range completion.RspToIDs {
		kernelID, ok := m.wgToKernel[wgID]
		if !ok {
			continue
		}

		delete(m.wgToKernel, wgID)

		launch := m.launches[kernelID]
		launch.outstanding--
		if launch.outstanding == 0 {
			m.rspsToSend = append(m.rspsToSend,
				m.makeLaunchRsp(launch.reqID, launch.reqSrc))
			delete(m.launches, kernelID)
		}
	}

	return true
}

func (m *cpMiddleware) makeLaunchRsp(
	reqID uint64,
	dst messaging.RemotePort,
) protocol.LaunchKernelRsp {
	return protocol.LaunchKernelRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   m.driverPort().AsRemote(),
			Dst:   dst,
			RspTo: reqID,
		},
	}
}
