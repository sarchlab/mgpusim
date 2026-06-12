package cp

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// A RequestCollection contains a single MemCopy Msg and the IDs of the
// Read/Write requests that correspond to it, as well as the number of
// remaining requests
type RequestCollection struct {
	superiorRequest       messaging.Msg
	subordinateRequestIDs []uint64
	subordinateCount      int
}

// decrementCountIfExists reduces the subordinate count if a specific ID is
// present in the list of subordinate IDs, returning true if it was and false
// if it was not
func (rqC *RequestCollection) decrementCountIfExists(id uint64) bool {
	for _, subID := range rqC.subordinateRequestIDs {
		if id == subID {
			rqC.subordinateCount--
			return true
		}
	}

	return false
}

// isFinished returns true if the subordinate count is zero (i.e. the superior
// request is finished processing)
func (rqC *RequestCollection) isFinished() bool {
	return rqC.subordinateCount == 0
}

func (rqC *RequestCollection) getSuperior() messaging.Msg {
	return rqC.superiorRequest
}

func (rqC *RequestCollection) getSuperiorID() uint64 {
	return rqC.superiorRequest.Meta().ID
}

// appendSubordinateID adds a message ID to the list and increases the count
func (rqC *RequestCollection) appendSubordinateID(id uint64) {
	rqC.subordinateRequestIDs = append(rqC.subordinateRequestIDs, id)
	rqC.subordinateCount++
}

// NewRequestCollection creates a RequestCollection for the given superior
// request.
func NewRequestCollection(
	superiorRequest messaging.Msg,
) *RequestCollection {
	rqC := new(RequestCollection)
	rqC.superiorRequest = superiorRequest
	rqC.subordinateCount = 0
	return rqC
}

// DMASpec contains the immutable configuration of the DMA engine.
type DMASpec struct {
	Freq timing.Freq `json:"freq"`

	// Log2AccessSize is the log2 of the number of bytes accessed by each
	// memory request that the DMA engine generates.
	Log2AccessSize uint64 `json:"log2_access_size"`

	// MaxRequestCount is the maximum number of memory-copy requests that the
	// DMA engine processes concurrently.
	MaxRequestCount uint64 `json:"max_request_count"`
}

// DMAState contains the mutable runtime data of the DMA engine. The in-flight
// request bookkeeping holds message values and lives on the middleware
// instead.
type DMAState struct{}

// DMAResources holds the shared references used by the DMA engine.
type DMAResources struct {
	// LocalDataSource maps addresses to the ports that can provide the data.
	LocalDataSource mem.AddressToPortMapper
}

// DMAComp is a DMAEngine component. A DMAEngine is responsible for accessing
// data that does not belong to the GPU that the DMAEngine works in.
//
// Ports (declared in Build, assigned externally):
//   - "ToCP": connects to the Command Processor.
//   - "ToMem": connects to the memory system.
type DMAComp = modeling.Component[DMASpec, DMAState, DMAResources]

// DMAEngine is an alias of DMAComp, kept for readability at use sites.
type DMAEngine = DMAComp

// dmaMiddleware implements the behavior of the DMA engine.
type dmaMiddleware struct {
	comp *DMAComp

	// TODO(akita5): state purity — the in-flight request bookkeeping holds
	// message values of mixed types, so it lives here rather than in the
	// component State. It is not checkpointable.
	processingReqs []*RequestCollection
	pendingReqs    []messaging.Msg
	toSendToMem    []messaging.Msg
	toSendToCP     []messaging.Msg
}

func (m *dmaMiddleware) toCP() messaging.Port {
	return m.comp.GetPortByName("ToCP")
}

func (m *dmaMiddleware) toMem() messaging.Port {
	return m.comp.GetPortByName("ToMem")
}

func (m *dmaMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.send(m.toCP(), &m.toSendToCP) || madeProgress
	madeProgress = m.send(m.toMem(), &m.toSendToMem) || madeProgress
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
	req := m.toMem().RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
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
	rsp memprotocol.DataReadyRsp,
) {
	req := m.removeReqFromPendingReqList(
		rsp.Meta().RspTo).(memprotocol.ReadReq)
	tracing.TraceReqFinalize(m.comp, req)

	found := false
	result := &RequestCollection{}
	for _, rc := range m.processingReqs {
		if rc.decrementCountIfExists(req.Meta().ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	processing := result.getSuperior().(protocol.MemCopyD2HReq)

	offset := req.Address - processing.SrcAddress
	copy(processing.DstBuffer[offset:], rsp.Data)

	if result.isFinished() {
		tracing.TraceReqComplete(m.comp, processing)
		m.removeReqFromProcessingReqList(processing.Meta().ID)

		rsp := protocol.GeneralRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   processing.Dst,
				Dst:   processing.Src,
				RspTo: processing.ID,
			},
		}
		m.toSendToCP = append(m.toSendToCP, rsp)
	}
}

func (m *dmaMiddleware) processDoneRsp(
	rsp memprotocol.WriteDoneRsp,
) {
	r := m.removeReqFromPendingReqList(rsp.Meta().RspTo)
	tracing.TraceReqFinalize(m.comp, r)

	found := false
	result := &RequestCollection{}
	for _, rc := range m.processingReqs {
		if rc.decrementCountIfExists(r.Meta().ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	if result.isFinished() {
		processing := result.getSuperior().(protocol.MemCopyH2DReq)
		tracing.TraceReqComplete(m.comp, processing)
		m.removeReqFromProcessingReqList(processing.Meta().ID)

		rsp := protocol.GeneralRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   processing.Dst,
				Dst:   processing.Src,
				RspTo: processing.ID,
			},
		}
		m.toSendToCP = append(m.toSendToCP, rsp)
	}
}

func (m *dmaMiddleware) removeReqFromPendingReqList(
	id uint64,
) messaging.Msg {
	var reqToRet messaging.Msg
	newList := make([]messaging.Msg, 0, len(m.pendingReqs)-1)
	for _, r := range m.pendingReqs {
		if r.Meta().ID == id {
			reqToRet = r
		} else {
			newList = append(newList, r)
		}
	}
	m.pendingReqs = newList

	if reqToRet == nil {
		panic("not found")
	}

	return reqToRet
}

func (m *dmaMiddleware) removeReqFromProcessingReqList(id uint64) {
	found := false
	newList := make([]*RequestCollection, 0, len(m.processingReqs)-1)
	for _, r := range m.processingReqs {
		if r.getSuperiorID() == id {
			found = true
		} else {
			newList = append(newList, r)
		}
	}
	m.processingReqs = newList

	if !found {
		panic("not found")
	}
}

func (m *dmaMiddleware) parseFromCP() bool {
	if uint64(len(m.processingReqs)) >= m.comp.Spec().MaxRequestCount {
		return false
	}

	req := m.toCP().RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(m.comp, req)

	rqC := NewRequestCollection(req)

	m.processingReqs = append(m.processingReqs, rqC)
	switch req := req.(type) {
	case protocol.MemCopyH2DReq:
		m.parseMemCopyH2D(req, rqC)
	case protocol.MemCopyD2HReq:
		m.parseMemCopyD2H(req, rqC)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (m *dmaMiddleware) parseMemCopyH2D(
	req protocol.MemCopyH2DReq,
	rqC *RequestCollection,
) {
	spec := m.comp.Spec()
	offset := uint64(0)
	lengthLeft := uint64(len(req.SrcBuffer))
	addr := req.DstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << spec.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << spec.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := m.comp.Resources().LocalDataSource.Find(addr)
		reqToBottom := memprotocol.WriteReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: m.toMem().AsRemote(),
				Dst: module,
			},
			Address: addr,
			Data:    req.SrcBuffer[offset : offset+length],
		}
		m.toSendToMem = append(m.toSendToMem, reqToBottom)
		m.pendingReqs = append(m.pendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.Meta().ID)

		tracing.TraceReqInitiate(m.comp, reqToBottom,
			tracing.MsgIDAtReceiver(req, m.comp))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (m *dmaMiddleware) parseMemCopyD2H(
	req protocol.MemCopyD2HReq,
	rqC *RequestCollection,
) {
	spec := m.comp.Spec()
	offset := uint64(0)
	lengthLeft := uint64(len(req.DstBuffer))
	addr := req.SrcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << spec.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << spec.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := m.comp.Resources().LocalDataSource.Find(addr)
		reqToBottom := memprotocol.ReadReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: m.toMem().AsRemote(),
				Dst: module,
			},
			Address:        addr,
			AccessByteSize: length,
		}
		m.toSendToMem = append(m.toSendToMem, reqToBottom)
		m.pendingReqs = append(m.pendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.Meta().ID)

		tracing.TraceReqInitiate(m.comp, reqToBottom,
			tracing.MsgIDAtReceiver(req, m.comp))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// defaultDMASpec provides the default configuration for the DMA engine.
var defaultDMASpec = DMASpec{
	Freq:            1 * timing.GHz,
	Log2AccessSize:  6,
	MaxRequestCount: 4,
}

// DefaultDMASpec returns a copy of the default DMA engine configuration.
func DefaultDMASpec() DMASpec {
	return defaultDMASpec
}

// DMAEngineBuilder builds DMA engines. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component declares its "ToCP" and "ToMem" ports; the
// port instances are supplied externally after Build with AssignPort.
type DMAEngineBuilder struct {
	spec      DMASpec
	registrar modeling.Registrar
	resources DMAResources
}

// MakeDMAEngineBuilder returns a new DMAEngineBuilder seeded with the default
// spec.
func MakeDMAEngineBuilder() DMAEngineBuilder {
	return DMAEngineBuilder{spec: defaultDMASpec}
}

// WithRegistrar wires the builder to a registrar.
func (b DMAEngineBuilder) WithRegistrar(
	reg modeling.Registrar,
) DMAEngineBuilder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultDMASpec() and
// tweak.
func (b DMAEngineBuilder) WithSpec(spec DMASpec) DMAEngineBuilder {
	b.spec = spec
	return b
}

// WithResources injects the address-to-port mapper that locates the modules
// holding the data.
func (b DMAEngineBuilder) WithResources(r DMAResources) DMAEngineBuilder {
	b.resources = r
	return b
}

// Build builds a new DMAComp. It declares the component's "ToCP" and "ToMem"
// ports; assign the port instances after Build with AssignPort.
func (b DMAEngineBuilder) Build(name string) *DMAComp {
	if b.registrar == nil {
		panic("cp: DMAEngineBuilder requires WithRegistrar")
	}

	comp := modeling.NewBuilder[DMASpec, DMAState, DMAResources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		WithResources(b.resources).
		Build(name)
	comp.State = DMAState{}

	comp.AddMiddleware(&dmaMiddleware{comp: comp})

	comp.DeclarePort("ToCP")
	comp.DeclarePort("ToMem", memprotocol.Requester)

	b.registrar.RegisterComponent(comp)

	return comp
}
