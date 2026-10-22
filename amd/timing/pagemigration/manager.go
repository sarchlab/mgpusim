package pagemigration

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// ManagerResources holds the live objects that the manager uses.
type ManagerResources struct {
	PageTable vm.PageTable
}

// Phases of a unit (see doc.go).
const (
	phaseIdle      = iota // not migrating; may be triggered if eligible
	phaseQueued           // in the destination GPU's software queue
	phaseService          // in a batch being serviced (SoftwareLatency)
	phaseShootdown        // TLBs being invalidated; all accesses held
	phaseDrain            // waiting for accesses in flight to the old frames
	phaseCopy             // engines moving the pages
)

// unitState is a migration unit (a page, or an aligned group of pages) and
// where it is in its migration.
type unitState struct {
	PID   vm.PID `json:"pid"`
	Index uint64 `json:"index"` // virtual address >> Log2Unit
	Phase int    `json:"phase"`
	Dst   uint64 `json:"dst"`

	// Faulters are the GPUs whose accesses wait during the queued and
	// service phases (on-fault policy).
	Faulters []uint64 `json:"faulters"`

	Migrations   int                   `json:"migrations"`
	EverMigrated bool                  `json:"ever_migrated"`
	LastEnd      timing.VTimeInPicoSec `json:"last_end"`
	Owners       uint64                `json:"owners"` // GPUs the unit has lived on

	Counts []uint32 `json:"counts"` // access counter per device ID

	Pages                []vm.Page `json:"pages"`
	NewPAddrs            []uint64  `json:"new_paddrs"`
	PendingChunks        int       `json:"pending_chunks"`
	PendingInvalidations int       `json:"pending_invalidations"`

	ReadyAt     timing.VTimeInPicoSec `json:"ready_at"`
	TriggerTime timing.VTimeInPicoSec `json:"trigger_time"`
	BlockStart  timing.VTimeInPicoSec `json:"block_start"`
	DrainStart  timing.VTimeInPicoSec `json:"drain_start"`
	CopyStart   timing.VTimeInPicoSec `json:"copy_start"`

	// Waiters are the names of the address translators that hold accesses
	// to the unit.
	Waiters []string `json:"waiters"`
}

func (u *unitState) blocks(dev uint64) bool {
	switch u.Phase {
	case phaseShootdown, phaseDrain, phaseCopy:
		return true
	case phaseQueued, phaseService:
		for _, f := range u.Faulters {
			if f == dev {
				return true
			}
		}
	}

	return false
}

// Frame statuses, for the invariant checks.
const (
	frameLive    = iota // mapped, accesses allowed
	frameFrozen         // being copied away: no access may be issued
	frameRetired        // migrated away: no access may ever reach it
)

// frameState is what the manager knows about a physical page. It is created
// by the first access issued to the frame.
type frameState struct {
	PID      vm.PID `json:"pid"`
	VAddr    uint64 `json:"vaddr"`    // page-aligned virtual address
	Inflight int    `json:"inflight"` // accesses issued, not yet answered
	DevMask  uint64 `json:"dev_mask"` // GPUs that accessed it
	Status   int    `json:"status"`
}

// handlerState is the software service of one GPU (the bottom half of the
// driver): it takes up to BatchSize queued units at a time and services them
// for SoftwareLatency.
type handlerState struct {
	Queue     []int                 `json:"queue"`
	Service   []int                 `json:"service"`
	Busy      bool                  `json:"busy"`
	BusyUntil timing.VTimeInPicoSec `json:"busy_until"`
}

// Stats are the migration statistics. Times are in picoseconds.
type Stats struct {
	Faults             uint64   `json:"faults"`
	CounterTriggers    uint64   `json:"counter_triggers"`
	Batches            uint64   `json:"batches"`
	Migrations         uint64   `json:"migrations"`
	NoopMigrations     uint64   `json:"noop_migrations"`
	PagesMigrated      uint64   `json:"pages_migrated"`
	BytesMigrated      uint64   `json:"bytes_migrated"`
	PingPong           uint64   `json:"ping_pong"`
	Pinned             uint64   `json:"pinned"`
	Shootdowns         uint64   `json:"shootdowns"`
	InvalidateReqs     uint64   `json:"invalidate_reqs"`
	RemoteAccesses     uint64   `json:"remote_accesses"`
	RemoteReads        uint64   `json:"remote_reads"`
	MigrationTimeSum   uint64   `json:"migration_time_sum"`
	BlockedTimeSum     uint64   `json:"blocked_time_sum"`
	DrainWaitSum       uint64   `json:"drain_wait_sum"`
	CopyTimeSum        uint64   `json:"copy_time_sum"`
	RemoteReadsByOwner []uint64 `json:"remote_reads_by_owner"`
	CheckedFrames      uint64   `json:"checked_frames"`
}

// gpuState is what the manager records about a GPU.
type gpuState struct {
	Registered bool                 `json:"registered"`
	Engine     messaging.RemotePort `json:"engine"`
}

// ManagerState is the mutable state of the manager. Units and frames are
// indexed so that the per-access hooks need one map lookup.
type ManagerState struct {
	Units     []unitState      `json:"units"`
	UnitIndex map[uint64]int   `json:"unit_index"`
	Frames    map[uint64]int   `json:"frames"` // by frame number
	FrameInfo []frameState     `json:"frame_info"`
	Handlers  []handlerState   `json:"handlers"` // by device ID
	Active    []int            `json:"active"`
	NextFrame []uint64         `json:"next_frame"` // by device ID
	GPUs      []gpuState       `json:"gpus"`       // by device ID
	OutInv    []InvalidateReq  `json:"out_inv"`
	OutCopies []CopyReq        `json:"out_copies"`
	InvUnits  map[uint64][]int `json:"inv_units"`  // by InvalidateReq ID
	CopyUnits map[uint64]int   `json:"copy_units"` // by CopyReq ID
	Stats     Stats            `json:"stats"`
}

// ManagerComp is the component that implements the manager.
type ManagerComp = modeling.Component[Spec, ManagerState, ManagerResources]

// Violation is the panic value of a broken migration invariant.
type Violation string

// GPU describes a GPU to the manager.
type GPU struct {
	DeviceID uint64

	// Engine is the page-migration engine of the GPU. It is nil when the
	// policy does not migrate.
	Engine *Engine

	// AddressTranslators are the translators that use the hooks of this GPU;
	// they are only read for the statistics.
	AddressTranslators []*addresstranslator.Comp

	// HoldsLine (used with Spec.Check) reports whether any L2 slice of the
	// GPU holds a line at a physical address or is fetching it.
	HoldsLine func(pAddr uint64) bool
}

// Manager decides what to migrate and runs the migrations. It plays the role
// of the driver's migration service: it is triggered by the hooks of the
// address translators and the RDMA engines, services batches after a
// software latency, and sends the shootdowns and copies to the engines of
// the GPUs.
//
// The hooks are direct calls into the manager, so page migration requires
// the serial engine.
type Manager struct {
	*ManagerComp

	mw *managerMW
}

// ManagerBuilder builds a Manager.
type ManagerBuilder struct {
	spec      Spec
	registrar modeling.Registrar
	resources ManagerResources
}

// MakeManagerBuilder returns a ManagerBuilder with the default configuration.
func MakeManagerBuilder() ManagerBuilder {
	return ManagerBuilder{spec: DefaultSpec()}
}

// WithRegistrar sets the registrar that provides the engine and registers
// the component.
func (b ManagerBuilder) WithRegistrar(reg modeling.Registrar) ManagerBuilder {
	b.registrar = reg
	return b
}

// WithSpec sets the configuration.
func (b ManagerBuilder) WithSpec(spec Spec) ManagerBuilder {
	b.spec = spec
	return b
}

// WithResources sets the page table.
func (b ManagerBuilder) WithResources(r ManagerResources) ManagerBuilder {
	b.resources = r
	return b
}

// Build builds the manager. When the policy migrates, it declares the
// "ToEngines" port, which must be connected to the "Remote" ports of the
// engines; assign the port instance after Build with AssignPort.
func (b ManagerBuilder) Build(name string) *Manager {
	if b.registrar == nil {
		panic("pagemigration: WithRegistrar is required")
	}

	if err := b.spec.Validate(); err != nil {
		panic(err)
	}

	comp := modeling.NewBuilder[Spec, ManagerState, ManagerResources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		WithResources(b.resources).
		Build(name)
	comp.State = ManagerState{
		UnitIndex: make(map[uint64]int),
		Frames:    make(map[uint64]int),
		InvUnits:  make(map[uint64][]int),
		CopyUnits: make(map[uint64]int),
	}

	m := &Manager{
		ManagerComp: comp,
		mw: &managerMW{
			comp:   comp,
			wakers: make(map[string]addresstranslator.Waker),
		},
	}
	m.mw.m = m

	comp.AddMiddleware(m.mw)

	if b.spec.Migrates() {
		comp.DeclarePort("ToEngines")
	}

	b.registrar.RegisterComponent(comp)

	return m
}

// RegisterGPU adds a GPU. Every GPU that runs accesses with the hooks of the
// manager must be registered before the simulation starts.
func (m *Manager) RegisterGPU(g GPU) {
	state := &m.State
	dev := int(g.DeviceID)

	if dev <= 0 || dev >= 64 {
		panic(fmt.Sprintf("page migration supports GPU IDs 1 to 63, got %d", dev))
	}

	for len(state.GPUs) <= dev {
		state.GPUs = append(state.GPUs, gpuState{})
		state.Handlers = append(state.Handlers, handlerState{})
		state.NextFrame = append(state.NextFrame, 0)
		state.Stats.RemoteReadsByOwner = append(
			state.Stats.RemoteReadsByOwner, 0)
	}

	if m.Spec().Migrates() && g.Engine == nil {
		panic("page migration: a migrating policy needs an engine per GPU")
	}

	state.GPUs[dev].Registered = true
	if g.Engine != nil {
		state.GPUs[dev].Engine = g.Engine.GetPortByName("Remote").AsRemote()
	}

	spec := m.Spec()
	state.NextFrame[dev] = g.DeviceID*spec.GPUMemSize + spec.FrameRegionOffset

	m.mw.gpus = append(m.mw.gpus, g)
}

// ATHook returns the hook for an address translator of GPU dev. Only the
// translators of vector memory accesses raise faults (canFault); scalar and
// instruction accesses are held and remapped but never trigger a migration.
func (m *Manager) ATHook(dev uint64, canFault bool) addresstranslator.MigrationHook {
	return &atHook{mw: m.mw, dev: dev, canFault: canFault}
}

// Stats returns the statistics.
func (m *Manager) Stats() Stats {
	return m.State.Stats
}

// Metric is one statistic of page migration.
type Metric struct {
	Location string
	What     string
	Value    float64
	Unit     string
}

// Metrics returns the configuration and the statistics of page migration.
func (m *Manager) Metrics() []Metric {
	var metrics []Metric

	add := func(what string, value float64, unit string) {
		metrics = append(metrics, Metric{
			Location: m.Name(), What: what, Value: value, Unit: unit,
		})
	}

	m.addCountMetrics(add)
	m.addTimeMetrics(add)
	m.addComponentMetrics(add)

	for dev, n := range m.State.Stats.RemoteReadsByOwner {
		if n > 0 {
			metrics = append(metrics, Metric{
				Location: fmt.Sprintf("GPU[%d]", dev),
				What:     "pm_remote_reads_served",
				Value:    float64(n),
				Unit:     "count",
			})
		}
	}

	return metrics
}

type addMetricFunc func(what string, value float64, unit string)

func (m *Manager) addCountMetrics(add addMetricFunc) {
	spec := m.Spec()
	s := m.State.Stats

	add("pm_policy_code", float64(spec.Policy), "")
	add("pm_unit_bytes", float64(uint64(1)<<spec.Log2Unit), "byte")

	for _, c := range []struct {
		what  string
		value uint64
	}{
		{"pm_faults", s.Faults},
		{"pm_counter_triggers", s.CounterTriggers},
		{"pm_batches", s.Batches},
		{"pm_migrations", s.Migrations},
		{"pm_noop_migrations", s.NoopMigrations},
		{"pm_pages_migrated", s.PagesMigrated},
		{"pm_bytes_migrated", s.BytesMigrated},
		{"pm_pingpong", s.PingPong},
		{"pm_pinned_units", s.Pinned},
		{"pm_shootdowns", s.Shootdowns},
		{"pm_invalidate_reqs", s.InvalidateReqs},
		{"pm_remote_accesses", s.RemoteAccesses},
		{"pm_remote_reads", s.RemoteReads},
		{"pm_units_touched", uint64(len(m.State.Units))},
		{"pm_checked_frames", s.CheckedFrames},
	} {
		add(c.what, float64(c.value), "count")
	}

	share, owner := m.HotOwnerShare()
	add("pm_hot_owner_read_share", share, "")
	add("pm_hot_owner_device", float64(owner), "")

	hist := m.MigrationsPerUnitHistogram()
	for k := 1; k < len(hist); k++ {
		if hist[k] > 0 {
			add(fmt.Sprintf("pm_units_migrated_%dx", k), float64(hist[k]), "count")
		}
	}
}

func (m *Manager) addTimeMetrics(add addMetricFunc) {
	s := m.State.Stats
	if s.Migrations == 0 {
		return
	}

	avg := func(sum uint64) float64 {
		return float64(sum) * 1e-12 / float64(s.Migrations)
	}

	add("pm_avg_migration_time", avg(s.MigrationTimeSum), "second")
	add("pm_avg_blocked_time", avg(s.BlockedTimeSum), "second")
	add("pm_avg_drain_wait", avg(s.DrainWaitSum), "second")
	add("pm_avg_copy_time", avg(s.CopyTimeSum), "second")
}

func (m *Manager) addComponentMetrics(add addMetricFunc) {
	var held, pulled, served uint64

	for _, g := range m.mw.gpus {
		for _, at := range g.AddressTranslators {
			held += at.State.NumHeldReqs
		}

		if g.Engine != nil {
			pulled += g.Engine.State.PulledBytes
			served += g.Engine.State.ServedBytes
		}
	}

	add("pm_held_accesses", float64(held), "count")
	add("pm_dma_pulled_bytes", float64(pulled), "byte")
	add("pm_dma_served_bytes", float64(served), "byte")
}

// HotOwnerShare returns the largest share of the remote reads served by one
// device and that device.
func (m *Manager) HotOwnerShare() (float64, uint64) {
	var total, best, owner uint64

	for d, n := range m.State.Stats.RemoteReadsByOwner {
		total += n
		if n > best {
			best, owner = n, uint64(d)
		}
	}

	if total == 0 {
		return 0, 0
	}

	return float64(best) / float64(total), owner
}

// MigrationsPerUnitHistogram returns, at index k, the number of units that
// migrated k times.
func (m *Manager) MigrationsPerUnitHistogram() []int {
	var hist []int

	for _, u := range m.State.Units {
		for len(hist) <= u.Migrations {
			hist = append(hist, 0)
		}

		hist[u.Migrations]++
	}

	return hist
}

// ManagerOf returns the Manager that implements component c, or nil if c is
// not a page-migration manager.
func ManagerOf(c any) *Manager {
	comp, ok := c.(*ManagerComp)
	if !ok {
		return nil
	}

	for _, mw := range comp.Middlewares() {
		if mmw, ok := mw.(*managerMW); ok {
			return mmw.m
		}
	}

	return nil
}
