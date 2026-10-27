package trace

import (
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/sarchlab/akita/v5/sim/modeling"
)

// BlockRef and WarpRef address immutable trace data in a shared Store. IDs are
// launch identities, not trace-file names (the same trace may launch repeatedly).
type BlockRef struct {
	Kernel uint64
	Block  int
}
type WarpRef struct {
	Kernel      uint64
	Block, Warp int
}

type storeState struct {
	NextID  uint64
	Kernels map[uint64]*KernelTrace
}

// Store owns the sole checkpointed copy of each active kernel trace. The driver
// admits isolated submissions and releases them after all device work completes.
// Reads borrow immutable data for the current operation only, never across a
// checkpoint restore. Host submission must not run concurrently with simulation.
type Store struct {
	mu    sync.RWMutex
	name  string
	state storeState
}

func NewStore(reg modeling.Simulation, name string) *Store {
	s := &Store{name: name, state: storeState{Kernels: make(map[uint64]*KernelTrace)}}
	reg.RegisterResource(s)
	return s
}
func (s *Store) Name() string { return s.name }
func (s *Store) SaveCheckpoint(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return modeling.WriteCheckpoint(w, struct{ Version int }{1}, s.state, nil)
}
func (s *Store) LoadCheckpoint(r io.Reader) error {
	var state storeState
	if err := modeling.ReadCheckpoint(r, struct{ Version int }{1}, &state, nil); err != nil {
		return err
	}
	for id, kernel := range state.Kernels {
		if id == 0 || id > state.NextID || kernel == nil {
			return fmt.Errorf("trace store: invalid kernel %d", id)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	return nil
}

// Add takes an isolated copy; later edits to the submitted trace cannot change
// queued or resident work. Each submission has one owner, even for equal traces.
func (s *Store) Add(kernel *KernelTrace) uint64 {
	if kernel == nil {
		panic("trace store: nil kernel")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.NextID++
	if s.state.NextID == 0 {
		panic("trace store: exhausted launch IDs")
	}
	copy := *kernel
	copy.Threadblocks = slices.Clone(kernel.Threadblocks)
	for bi := range copy.Threadblocks {
		b := &copy.Threadblocks[bi]
		b.Warps = slices.Clone(b.Warps)
		for wi := range b.Warps {
			w := &b.Warps[wi]
			w.Instructions = slices.Clone(w.Instructions)
			for ii := range w.Instructions {
				i := &w.Instructions[ii]
				i.SrcRegs = slices.Clone(i.SrcRegs)
				i.DestRegs = slices.Clone(i.DestRegs)
				i.MemAddressSuffix2 = slices.Clone(i.MemAddressSuffix2)
			}
		}
	}
	if s.state.Kernels == nil {
		s.state.Kernels = make(map[uint64]*KernelTrace)
	}
	s.state.Kernels[s.state.NextID] = &copy
	return s.state.NextID
}
func (s *Store) Release(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Kernels[id] == nil {
		panic(fmt.Sprintf("trace store: unknown kernel %d", id))
	}
	delete(s.state.Kernels, id)
}
func (s *Store) Kernel(id uint64) *KernelTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := s.state.Kernels[id]
	if k == nil {
		panic(fmt.Sprintf("trace store: unknown kernel %d", id))
	}
	return k
}
func (s *Store) Block(ref BlockRef) *ThreadblockTrace {
	return &s.Kernel(ref.Kernel).Threadblocks[ref.Block]
}
func (s *Store) Warp(ref WarpRef) *WarpTrace {
	return &s.Kernel(ref.Kernel).Threadblocks[ref.Block].Warps[ref.Warp]
}
