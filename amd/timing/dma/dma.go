package dma

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// A RequestCollection contains a single MemCopy Msg and the IDs of the
// Read/Write requests that correspond to it, as well as the number of
// remaining requests
type RequestCollection struct {
	RecvTaskID            uint64
	Data                  []byte
	SuperiorRequest       messaging.Msg
	SubordinateRequestIDs []uint64
	SubordinateCount      int
}

// decrementCountIfExists reduces the subordinate count if a specific ID is
// present in the list of subordinate IDs, returning true if it was and false
// if it was not
func (rqC *RequestCollection) decrementCountIfExists(id uint64) bool {
	for _, subID := range rqC.SubordinateRequestIDs {
		if id == subID {
			rqC.SubordinateCount--
			return true
		}
	}

	return false
}

// isFinished returns true if the subordinate count is zero (i.e. the superior
// request is finished processing)
func (rqC *RequestCollection) isFinished() bool {
	return rqC.SubordinateCount == 0
}

func (rqC *RequestCollection) getSuperior() messaging.Msg {
	return rqC.SuperiorRequest
}

func (rqC *RequestCollection) getSuperiorID() uint64 {
	return rqC.SuperiorRequest.ID
}

// appendSubordinateID adds a message ID to the list and increases the count
func (rqC *RequestCollection) appendSubordinateID(id uint64) {
	rqC.SubordinateRequestIDs = append(rqC.SubordinateRequestIDs, id)
	rqC.SubordinateCount++
}

// NewRequestCollection creates a RequestCollection for the given superior
// request.
func NewRequestCollection(
	SuperiorRequest messaging.Msg,
) *RequestCollection {
	rqC := new(RequestCollection)
	rqC.SuperiorRequest = SuperiorRequest
	rqC.SubordinateCount = 0
	return rqC
}

// Spec contains the immutable configuration of the DMA engine.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Log2AccessSize is the log2 of the number of bytes accessed by each
	// memory request that the DMA engine generates.
	Log2AccessSize uint64 `json:"log2_access_size"`

	// MaxRequestCount is the maximum number of memory-copy requests that the
	// DMA engine processes concurrently.
	MaxRequestCount uint64 `json:"max_request_count"`
}

// State contains all queued requests, partial copyback bytes and task handles.
type State struct {
	ProcessingReqs []RequestCollection
	PendingReqs    []messaging.Msg
	ToSendToMem    []messaging.Msg
	ToSendToCP     []messaging.Msg
	PendingTaskIDs map[uint64]uint64
}

func (s *State) Initialize(*Comp) { s.PendingTaskIDs = make(map[uint64]uint64) }

// Resources holds the shared references used by the DMA engine.
type Resources struct {
	// LocalDataSource maps addresses to the ports that can provide the data.
	LocalDataSource mem.AddressToPortMapper
}

// Comp is a DMAEngine component. A DMAEngine is responsible for accessing
// data that does not belong to the GPU that the DMAEngine works in.
//
// Ports (declared in Build, assigned externally):
//   - "ToCP": connects to the Command Processor.
//   - "ToMem": connects to the memory system.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	ToCP  messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.dma"`
	ToMem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}
type Middlewares struct{ DMA *dmaMiddleware }

func (m *Middlewares) Initialize(c *Comp)         { m.DMA = &dmaMiddleware{comp: c} }
func (m *dmaMiddleware) Handle(timing.Event) bool { return m.Tick() }

// dmaMiddleware implements the behavior of the DMA engine.
type dmaMiddleware struct {
	comp *Comp
}

func (m *dmaMiddleware) toCP() messaging.Port {
	return m.comp.Ports.ToCP
}

func (m *dmaMiddleware) toMem() messaging.Port {
	return m.comp.Ports.ToMem
}

func (m *dmaMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.send(m.toCP(), &m.comp.State.ToSendToCP) || madeProgress
	madeProgress = m.send(m.toMem(), &m.comp.State.ToSendToMem) || madeProgress
	madeProgress = m.parseFromMem() || madeProgress
	madeProgress = m.parseFromCP() || madeProgress

	return madeProgress
}

func (m *dmaMiddleware) send(
	port messaging.Port,
	reqs *[]messaging.Msg,
) bool {
	if len(*reqs) == 0 {
		return false
	}

	if !port.CanSend() {
		return false
	}

	port.Send((*reqs)[0])
	*reqs = (*reqs)[1:]

	return true
}

func (m *dmaMiddleware) parseFromMem() bool {
	req, ok := m.toMem().RetrieveIncoming()
	if !ok {
		return false
	}

	switch req.Payload.(type) {
	case memprotocol.DataReadyRsp:

		m.processDataReadyRsp(req)
	case memprotocol.WriteDoneRsp:

		m.processDoneRsp(req)
	default:

		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (m *dmaMiddleware) processDataReadyRsp(
	rsp messaging.Msg,
) {
	req := m.removeReqFromPendingReqList(
		rsp.RspTo)
	m.finishBottomRequest(req.ID)

	found := false
	result := &RequestCollection{}
	for i := range m.comp.State.ProcessingReqs {
		rc := &m.comp.State.ProcessingReqs[i]
		if rc.decrementCountIfExists(req.ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	processing := result.getSuperior()

	offset := req.Payload.(memprotocol.ReadReq).Address - processing.Payload.(protocol.MemCopyD2HReq).SrcAddress
	copy(result.Data[offset:], rsp.Payload.(memprotocol.DataReadyRsp).Data)

	if result.isFinished() {
		// All bottom reads have returned: mark the end of the wait for memory.
		m.comp.TaskTracker().
			AddMilestone(tasks.Milestone{TaskID: result.RecvTaskID, Kind: tasks.MilestoneKindData, What: "ToMem"})
		m.comp.TaskTracker().EndReqIn(result.RecvTaskID)
		m.removeReqFromProcessingReqList(processing.ID)

		rsp := messaging.Msg{ID: m.comp.NewID(),
			Src:   processing.Dst,
			Dst:   processing.Src,
			RspTo: processing.ID, Payload: protocol.MemCopyD2HRsp{Data: result.Data}}

		m.comp.State.ToSendToCP = append(m.comp.State.ToSendToCP, rsp)
	}
}

func (m *dmaMiddleware) processDoneRsp(
	rsp messaging.Msg,
) {
	r := m.removeReqFromPendingReqList(rsp.RspTo)
	m.finishBottomRequest(r.ID)

	found := false
	result := &RequestCollection{}
	for i := range m.comp.State.ProcessingReqs {
		rc := &m.comp.State.ProcessingReqs[i]
		if rc.decrementCountIfExists(r.ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	if result.isFinished() {
		processing := result.getSuperior()
		// All bottom writes have completed: mark the end of the wait for memory.
		m.comp.TaskTracker().
			AddMilestone(tasks.Milestone{TaskID: result.RecvTaskID, Kind: tasks.MilestoneKindData, What: "ToMem"})
		m.comp.TaskTracker().EndReqIn(result.RecvTaskID)
		m.removeReqFromProcessingReqList(processing.ID)

		rsp := messaging.Msg{ID: m.comp.NewID(),
			Src:   processing.Dst,
			Dst:   processing.Src,
			RspTo: processing.ID, Payload: protocol.GeneralRsp{}}

		m.comp.State.ToSendToCP = append(m.comp.State.ToSendToCP, rsp)
	}
}

func (m *dmaMiddleware) removeReqFromPendingReqList(
	id uint64,
) messaging.Msg {
	var reqToRet messaging.Msg
	newList := make([]messaging.Msg, 0, len(m.comp.State.PendingReqs)-1)
	for _, r := range m.comp.State.PendingReqs {
		if r.ID == id {
			reqToRet = r
		} else {
			newList = append(newList, r)
		}
	}
	m.comp.State.PendingReqs = newList

	if reqToRet.ID == 0 {
		panic("not found")
	}

	return reqToRet
}

func (m *dmaMiddleware) removeReqFromProcessingReqList(id uint64) {
	found := false
	newList := make([]RequestCollection, 0, len(m.comp.State.ProcessingReqs)-1)
	for _, r := range m.comp.State.ProcessingReqs {
		if r.getSuperiorID() == id {
			found = true
		} else {
			newList = append(newList, r)
		}
	}
	m.comp.State.ProcessingReqs = newList

	if !found {
		panic("not found")
	}
}

func (m *dmaMiddleware) parseFromCP() bool {
	if uint64(len(m.comp.State.ProcessingReqs)) >= m.comp.Spec.MaxRequestCount {
		return false
	}

	req, ok := m.toCP().PeekIncoming()
	if !ok {
		return false
	}

	// A processing slot was free, so this request is admitted now. If it had to
	// wait in the incoming buffer for a slot, this marks the resolution of that
	// wait on the buffer task (the DBTracer collapses it when there was none).
	m.toCP().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".processing-slot"))

	m.toCP().RetrieveIncoming()

	rqC := NewRequestCollection(req)
	rqC.RecvTaskID = m.comp.TaskTracker().StartReqIn(m.comp, req)

	switch req.Payload.(type) {
	case protocol.MemCopyH2DReq:

		m.parseMemCopyH2D(req, rqC)
	case protocol.MemCopyD2HReq:

		m.parseMemCopyD2H(req, rqC)
	default:

		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	m.comp.State.ProcessingReqs = append(m.comp.State.ProcessingReqs, *rqC)
	return true
}

func (m *dmaMiddleware) parseMemCopyH2D(
	req messaging.Msg,
	rqC *RequestCollection,
) {
	spec := m.comp.Spec
	offset := uint64(0)
	lengthLeft := uint64(len(req.Payload.(protocol.MemCopyH2DReq).SrcBuffer))
	addr := req.Payload.(protocol.MemCopyH2DReq).DstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << spec.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << spec.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := m.comp.Resources.LocalDataSource.Find(addr)
		reqToBottom := messaging.Msg{ID: m.comp.NewID(),
			Src: m.toMem().AsRemote(),
			Dst: module, Payload: memprotocol.WriteReq{Address: addr,
				Data: req.Payload.(protocol.MemCopyH2DReq).SrcBuffer[offset : offset+length]}}

		m.comp.State.ToSendToMem = append(m.comp.State.ToSendToMem, reqToBottom)
		m.comp.State.PendingReqs = append(m.comp.State.PendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.ID)

		m.comp.State.PendingTaskIDs[reqToBottom.ID] = m.comp.TaskTracker().
			StartReqOut(m.comp, reqToBottom, rqC.RecvTaskID)

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (m *dmaMiddleware) parseMemCopyD2H(
	req messaging.Msg,
	rqC *RequestCollection,
) {
	spec := m.comp.Spec
	offset := uint64(0)
	lengthLeft := req.Payload.(protocol.MemCopyD2HReq).ByteSize
	rqC.Data = make([]byte, lengthLeft)
	addr := req.Payload.(protocol.MemCopyD2HReq).SrcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << spec.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << spec.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := m.comp.Resources.LocalDataSource.Find(addr)
		reqToBottom := messaging.Msg{ID: m.comp.NewID(),
			Src: m.toMem().AsRemote(),
			Dst: module, Payload: memprotocol.ReadReq{Address: addr,
				AccessByteSize: length}}

		m.comp.State.ToSendToMem = append(m.comp.State.ToSendToMem, reqToBottom)
		m.comp.State.PendingReqs = append(m.comp.State.PendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.ID)

		m.comp.State.PendingTaskIDs[reqToBottom.ID] = m.comp.TaskTracker().
			StartReqOut(m.comp, reqToBottom, rqC.RecvTaskID)

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// DefaultSpec returns the default DMA configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz, Log2AccessSize: 6, MaxRequestCount: 4}
}

// Builder builds DMA engines. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation and
// WithResources. The component declares its "ToCP" and "ToMem" ports; the
// port instances are supplied externally after Build with BindPort.
type Builder struct {
	spec      Spec
	registrar modeling.Simulation
	resources Resources
}

// MakeBuilder returns a new Builder seeded with the default
// spec.
func MakeBuilder() Builder {
	return Builder{spec: DefaultSpec()}
}

// WithSimulation wires the builder to a registrar.
func (b Builder) WithSimulation(
	reg modeling.Simulation,
) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and
// tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the address-to-port mapper that locates the modules
// holding the data.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new Comp. It declares the component's "ToCP" and "ToMem"
// ports; assign the port instances after Build with BindPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("dma: Builder requires WithSimulation")
	}

	return modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(b.resources).Build(name)
}
func (m *dmaMiddleware) finishBottomRequest(id uint64) {
	m.comp.TaskTracker().EndReqOut(m.comp.State.PendingTaskIDs[id])
	delete(m.comp.State.PendingTaskIDs, id)
}
