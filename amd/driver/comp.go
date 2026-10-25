package driver

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const GPUPortName = "GPU"

// AllocatorType selects physical-memory allocation for this platform.
type AllocatorType int

const (
	AllocatorDefault AllocatorType = iota
	AllocatorBuddy
)

type DeviceProperties struct {
	CUCount  int    `json:"cu_count"`
	DRAMSize uint64 `json:"dram_size"`
}

// Spec is the immutable driver and memory-allocation configuration.
type Spec struct {
	Freq               timing.Freq            `json:"freq"`
	Log2PageSize       uint64                 `json:"log2_page_size"`
	UseMagicMemoryCopy bool                   `json:"use_magic_memory_copy"`
	D2HCycles          int                    `json:"d2h_cycles"`
	H2DCycles          int                    `json:"h2d_cycles"`
	Allocator          AllocatorType          `json:"allocator"`
	GPUs               []messaging.RemotePort `json:"gpus"`
	Devices            []DeviceProperties     `json:"devices"`
}

// Execution holds the sole mutable copy of a command already taken from Runtime.
type Execution struct {
	Queue    CommandQueue
	Command  Command
	Requests []messaging.Msg
}

// State contains only simulated execution state, never host continuation data.
type State struct {
	Executing                  []Execution
	RequestsToSend             []messaging.Msg
	CopyAwaiting               []messaging.Msg
	CopyCyclesLeft             int
	CommandTasks, RequestTasks map[uint64]uint64
}

func (s *State) Initialize(*Comp) {
	s.CopyCyclesLeft = -1
	s.CommandTasks = make(map[uint64]uint64)
	s.RequestTasks = make(map[uint64]uint64)
}

// Resources are live shared objects supplied before construction.
type Resources struct {
	Runtime       *Runtime
	PageTable     *vm.PageTable
	GlobalStorage *mem.Storage
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	GPU messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.driver"`
}
type Middlewares struct{ Execute *executionMW }

func (m *Middlewares) Initialize(c *Comp) { m.Execute = &executionMW{comp: c} }
