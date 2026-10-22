package pagemigration

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// EngineSpec is the configuration of the page-migration engine of one GPU.
type EngineSpec struct {
	Freq timing.Freq `json:"freq"`

	// PullWindow is the number of chunks that the engine pulls at the same
	// time as a destination. MemWindow is the number of line accesses that
	// the engine keeps in flight to its L2 (as a source) and to its DRAM (as
	// a destination). LineBytes is the size of one line access.
	PullWindow int    `json:"pull_window"`
	MemWindow  int    `json:"mem_window"`
	LineBytes  uint64 `json:"line_bytes"`

	// TLBs are the Control ports of every TLB of the GPU. A shootdown pauses
	// each TLB, invalidates the migrating pages, and enables it again.
	TLBs []messaging.RemotePort `json:"tlbs"`
}

// EngineSpec returns the engine configuration that matches the manager
// configuration. The TLBs are set by the GPU builder.
func (s Spec) EngineSpec() EngineSpec {
	return EngineSpec{
		Freq:       s.Freq,
		PullWindow: s.PullWindow,
		MemWindow:  s.MemWindow,
		LineBytes:  s.LineBytes,
	}
}

// EngineResources holds the address mappers of the engine.
type EngineResources struct {
	// L2Mapper finds the L2 slice of a local physical address. A source
	// reads the pages through the L2, so dirty lines are copied.
	L2Mapper mem.AddressToPortMapper

	// DRAMMapper finds the memory controller of a local physical address. A
	// destination writes the new frames directly to its DRAM, around the
	// L2: a new frame was never accessed, so no cache holds any line of it,
	// and writing around the L2 leaves no dirty lines that a later
	// functional copy by the driver could be shadowed by.
	DRAMMapper mem.AddressToPortMapper
}

// copyState is a CopyReq at the destination engine.
type copyState struct {
	ReqID     uint64               `json:"req_id"`
	Requester messaging.RemotePort `json:"requester"`
	Source    messaging.RemotePort `json:"source"`
	SrcPAddr  uint64               `json:"src_paddr"`
	DstPAddr  uint64               `json:"dst_paddr"`
	Size      uint64               `json:"size"`
	LinesLeft int                  `json:"lines_left"`
}

// servingState is a PullReq at the source engine.
type servingState struct {
	Requester messaging.RemotePort `json:"requester"`
	Data      []byte               `json:"data"`
	LinesLeft int                  `json:"lines_left"`
}

// lineReadState locates the line that a read returns in a pull.
type lineReadState struct {
	PullID uint64 `json:"pull_id"`
	Offset uint64 `json:"offset"`
}

// Steps of the shootdown sequence of one TLB.
const (
	tlbIdle = iota
	tlbPausing
	tlbInvalidating
	tlbEnabling
)

// EngineState is the mutable state of the engine.
type EngineState struct {
	// Destination side: copies waiting to be pulled, pulls in flight (by
	// PullReq ID), pulled chunks being written (by PullReq ID), DRAM writes
	// to send and the pull that each write belongs to, and the finished
	// copies to report.
	Copies      []copyState             `json:"copies"`
	Pulling     map[uint64]copyState    `json:"pulling"`
	Landing     map[uint64]copyState    `json:"landing"`
	Writes      []memprotocol.WriteReq  `json:"writes"`
	WriteToPull map[uint64]uint64       `json:"write_to_pull"`
	CopyRsps    []CopyRsp               `json:"copy_rsps"`
	Serving     map[uint64]servingState `json:"serving"`

	// Source side: L2 reads to send, the pull that each read belongs to,
	// and the pull responses to send.
	Reads      []memprotocol.ReadReq    `json:"reads"`
	ReadToPull map[uint64]lineReadState `json:"read_to_pull"`
	PullRsps   []PullRsp                `json:"pull_rsps"`

	L2Inflight   int `json:"l2_inflight"`
	DRAMInflight int `json:"dram_inflight"`

	// Shootdown: the invalidations to perform (the first one is in
	// progress), the step of each TLB in it, the number of TLBs that have
	// not finished it, the control requests to send, and the finished
	// invalidations to report.
	Invalidations  []InvalidateReq          `json:"invalidations"`
	TLBSteps       []int                    `json:"tlb_steps"`
	TLBsLeft       int                      `json:"tlbs_left"`
	CtrlReqs       []memcontrolprotocol.Req `json:"ctrl_reqs"`
	InvalidateRsps []InvalidateRsp          `json:"invalidate_rsps"`

	// Statistics.
	PulledBytes uint64 `json:"pulled_bytes"`
	ServedBytes uint64 `json:"served_bytes"`
}

// Engine is the page-migration engine of one GPU. It performs the work of a
// migration that happens on its GPU:
//
//   - shootdown: on an InvalidateReq, it pauses each TLB of the GPU,
//     invalidates the migrating pages, and enables the TLB again (the
//     memcontrolprotocol verbs, sent on the "TLBControl" port);
//   - copy, as a destination: on a CopyReq, it pulls the chunk from the
//     engine of the source GPU (at most PullWindow chunks in flight) and
//     writes it to its DRAM;
//   - copy, as a source: on a PullReq, it reads the chunk through its L2
//     and returns it.
//
// Ports: "Remote" (to the manager and the other engines), "L2" (reads of the
// local L2 slices), "DRAM" (writes to the local memory controllers), and
// "TLBControl" (to the Control ports of the TLBs).
type Engine = modeling.Component[EngineSpec, EngineState, EngineResources]

// EngineBuilder builds page-migration engines.
type EngineBuilder struct {
	spec      EngineSpec
	registrar modeling.Registrar
	resources EngineResources
}

// MakeEngineBuilder returns an EngineBuilder with the default configuration.
func MakeEngineBuilder() EngineBuilder {
	return EngineBuilder{spec: DefaultSpec().EngineSpec()}
}

// WithRegistrar sets the registrar that provides the engine and registers
// the component.
func (b EngineBuilder) WithRegistrar(reg modeling.Registrar) EngineBuilder {
	b.registrar = reg
	return b
}

// WithSpec sets the configuration.
func (b EngineBuilder) WithSpec(spec EngineSpec) EngineBuilder {
	b.spec = spec
	return b
}

// WithResources sets the address mappers.
func (b EngineBuilder) WithResources(r EngineResources) EngineBuilder {
	b.resources = r
	return b
}

// Build builds an engine. It declares the "Remote", "L2", "DRAM", and
// "TLBControl" ports; assign the port instances after Build with AssignPort.
func (b EngineBuilder) Build(name string) *Engine {
	if b.registrar == nil {
		panic("pagemigration: WithRegistrar is required")
	}

	comp := modeling.NewBuilder[EngineSpec, EngineState, EngineResources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		WithResources(b.resources).
		Build(name)
	comp.State = EngineState{
		Pulling:     make(map[uint64]copyState),
		Landing:     make(map[uint64]copyState),
		WriteToPull: make(map[uint64]uint64),
		Serving:     make(map[uint64]servingState),
		ReadToPull:  make(map[uint64]lineReadState),
	}

	comp.AddMiddleware(&engineMW{comp: comp})

	comp.DeclarePort("Remote")
	comp.DeclarePort("L2", memprotocol.Requester)
	comp.DeclarePort("DRAM", memprotocol.Requester)
	comp.DeclarePort("TLBControl", memcontrolprotocol.Requester)

	b.registrar.RegisterComponent(comp)

	return comp
}
