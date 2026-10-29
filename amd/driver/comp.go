package driver

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const GPUPortName = "GPU"

// PageMigrationPortName names the port that receives the fault notifications
// and the command processors' migration answers.
const PageMigrationPortName = "PageMigration"

// Spec is the immutable simulated protocol and timing configuration.
type Spec struct {
	Freq               timing.Freq            `json:"freq"`
	UseMagicMemoryCopy bool                   `json:"use_magic_memory_copy"`
	D2HCycles          int                    `json:"d2h_cycles"`
	H2DCycles          int                    `json:"h2d_cycles"`
	GPUs               []messaging.RemotePort `json:"gpus"`

	// PageMigration holds the migration parameters. MigrationAgents are the
	// "PageMigration" ports of the GPUs' command processors, by GPU.
	PageMigration   PageMigrationSpec      `json:"page_migration"`
	MigrationAgents []messaging.RemotePort `json:"migration_agents"`
}

// Execution holds the sole mutable copy of a command already taken from gpuruntime.Runtime.
type Execution struct {
	Queue    gpuruntime.CommandQueue
	Command  gpuruntime.Command
	Requests []messaging.Msg

	// WaitsForMigration is set on a memory copy that has not started
	// because a unit it uses is migrating.
	WaitsForMigration bool
}

// State contains only simulated execution state, never host continuation data.
type State struct {
	Executing                  []Execution
	RequestsToSend             []messaging.Msg
	CopyAwaiting               []messaging.Msg
	CopyCyclesLeft             int
	CommandTasks, RequestTasks map[uint64]uint64

	// PageMigration is the state of the page-migration middleware.
	PageMigration PageMigrationState
}

func (s *State) Initialize(*Comp) {
	s.CopyCyclesLeft = -1
	s.CommandTasks = make(map[uint64]uint64)
	s.RequestTasks = make(map[uint64]uint64)
	s.PageMigration = newPageMigrationState()
}

// Resources are live shared objects supplied before construction.
type Resources struct {
	Runtime       *gpuruntime.Runtime
	PageTable     *vm.PageTable
	GlobalStorage *mem.Storage
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	GPU messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.driver"`

	// PageMigration receives the MMU's fault notifications and the command
	// processors' answers, and sends the migration commands.
	PageMigration messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.driver"`
}
type Middlewares struct {
	Execute   *executionMW
	Migration *migrationMW
}

func (m *Middlewares) Initialize(c *Comp) {
	if err := c.Spec.PageMigration.validate(len(c.Spec.GPUs), len(c.Spec.MigrationAgents)); err != nil {
		panic(err)
	}

	m.Execute = &executionMW{comp: c}
	m.Migration = &migrationMW{comp: c}
}
