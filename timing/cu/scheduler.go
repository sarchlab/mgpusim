package cu

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
type Scheduler struct {
	*core.BasicComponent
	sync.Mutex

	WfPools []*WavefrontPool

	MappedWGs []*timing.MapWGReq
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(name string) *Scheduler {
	s := new(Scheduler)
	s.BasicComponent = core.NewBasicComponent(name)
	s.WfPools = make([]*WavefrontPool, 0, 4)
	for i := 0; i < 4; i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool())
	}
	return s
}

// Recv function process the incoming requests
func (s *Scheduler) Recv(req core.Req) *core.Error {
	s.Lock()
	defer s.Unlock()

	switch req := req.(type) {
	case *timing.MapWGReq:
		return s.processMapWGReq(req)
	default:
		log.Panicf("Unable to process req %s", reflect.TypeOf(req))
	}

	return nil
}

func (s *Scheduler) processMapWGReq(req *timing.MapWGReq) *core.Error {
	return nil
}

// FetchInfo keeps record of the information of a fetch action
type FetchInfo struct {
	Buf []byte
	Wf  *Wavefront
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC uint64
}

// A WavefrontPool holds the wavefronts that will be scheduled in one SIMD
// unit
type WavefrontPool struct {
	capacity int

	Wfs         []*Wavefront
	FetchBuffer []*FetchInfo
}

// NewWavefrontPool creates and returns a new WavefrontPool
func NewWavefrontPool() *WavefrontPool {
	p := new(WavefrontPool)

	p.Wfs = make([]*Wavefront, 0, 0)

	return p
}

// A FetchArbitrator can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbitrator interface {
}

// An IssueArbitrator decides which wavefront can issue instruction
type IssueArbitrator interface {
}
