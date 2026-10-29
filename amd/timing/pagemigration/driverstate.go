package pagemigration

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Phases of a unit (see doc.go).
const (
	phaseIdle      = iota // nothing to do; may be triggered
	phaseQueued           // in the software queue of the destination GPU
	phaseService          // in a batch being serviced (SoftwareLatency)
	phaseHold             // waiting for the GPUs to hold the pages
	phaseCopy             // the data mover of the destination copies
	phaseShootdown        // the GPUs drop the old translations
	phaseRelease          // waiting for the GPUs to release the pages
)

// unitState is a migration unit (a page, or an aligned group of pages) and
// where it is in its service.
type unitState struct {
	PID   vm.PID `json:"pid"`
	Index uint64 `json:"index"` // virtual address >> Log2Unit
	Phase int    `json:"phase"`
	Dst   uint64 `json:"dst"`

	// Faulters are the GPUs whose accesses wait on a fault of the unit and
	// must be released by this service. LateFaults are the GPUs whose
	// faults arrived when they could not join this service; they are
	// handled as new faults when the unit becomes idle.
	Faulters   []uint64 `json:"faulters"`
	LateFaults []uint64 `json:"late_faults"`

	// Migrate is set when the service moves pages; otherwise it only maps
	// the unit remotely or lets the faulted accesses go. Targets are the
	// GPUs that hold and release the pages at VAddrs. Pages are the pages
	// that move and NewPAddrs their new frames.
	Migrate     bool      `json:"migrate"`
	Targets     []uint64  `json:"targets"`
	VAddrs      []uint64  `json:"vaddrs"`
	Pages       []vm.Page `json:"pages"`
	NewPAddrs   []uint64  `json:"new_paddrs"`
	PendingAcks int       `json:"pending_acks"`

	Migrations   int                   `json:"migrations"`
	EverMigrated bool                  `json:"ever_migrated"`
	Pinned       bool                  `json:"pinned"`
	LastEnd      timing.VTimeInPicoSec `json:"last_end"`
	Owners       uint64                `json:"owners"` // GPUs the unit has lived on

	TriggerTime    timing.VTimeInPicoSec `json:"trigger_time"`
	HoldStart      timing.VTimeInPicoSec `json:"hold_start"`
	CopyStart      timing.VTimeInPicoSec `json:"copy_start"`
	ShootdownStart timing.VTimeInPicoSec `json:"shootdown_start"`
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

// gpuState is what the driver records about a GPU.
type gpuState struct {
	Registered bool                 `json:"registered"`
	CP         messaging.RemotePort `json:"cp"`
}

// pageRef locates the page of a frame.
type pageRef struct {
	PID   vm.PID `json:"pid"`
	VAddr uint64 `json:"vaddr"`
}

// remoteMapState is a unit mapped remotely (its pages pinned) until the end
// of its cooldown.
type remoteMapState struct {
	Unit  int                   `json:"unit"`
	Until timing.VTimeInPicoSec `json:"until"`
}

// Stats are the migration statistics. Times are in picoseconds.
type Stats struct {
	Faults               uint64 `json:"faults"`
	CounterNotifications uint64 `json:"counter_notifications"`
	CounterTriggers      uint64 `json:"counter_triggers"`
	Batches              uint64 `json:"batches"`
	Services             uint64 `json:"services"`
	Migrations           uint64 `json:"migrations"`
	NoopServices         uint64 `json:"noop_services"`
	RemoteMaps           uint64 `json:"remote_maps"`
	PagesMigrated        uint64 `json:"pages_migrated"`
	BytesMigrated        uint64 `json:"bytes_migrated"`
	PingPong             uint64 `json:"ping_pong"`
	Pinned               uint64 `json:"pinned"`
	Shootdowns           uint64 `json:"shootdowns"`
	HoldReqs             uint64 `json:"hold_reqs"`
	MigrationTimeSum     uint64 `json:"migration_time_sum"`
	BlockedTimeSum       uint64 `json:"blocked_time_sum"`
	HoldTimeSum          uint64 `json:"hold_time_sum"`
	CopyTimeSum          uint64 `json:"copy_time_sum"`
	ShootdownTimeSum     uint64 `json:"shootdown_time_sum"`
}

// DriverState is the page-migration state of the driver.
type DriverState struct {
	GPUs      []gpuState     `json:"gpus"` // by device ID
	Units     []unitState    `json:"units"`
	UnitIndex map[uint64]int `json:"unit_index"`
	Handlers  []handlerState `json:"handlers"` // by device ID
	NextFrame []uint64       `json:"next_frame"`

	// Frames caches the page of each frame that a notification named, by
	// frame number.
	Frames map[uint64]pageRef `json:"frames"`

	// Pending maps the ID of each request in flight to its unit.
	Pending map[uint64]int `json:"pending"`

	RemoteMaps []remoteMapState `json:"remote_maps"`

	// Requests waiting to be sent.
	OutHolds      []HoldReq      `json:"out_holds"`
	OutCopies     []CopyReq      `json:"out_copies"`
	OutShootdowns []ShootdownReq `json:"out_shootdowns"`
	OutReleases   []ReleaseReq   `json:"out_releases"`

	Stats Stats `json:"stats"`
}

// NewDriverState returns an empty DriverState.
func NewDriverState() DriverState {
	return DriverState{
		UnitIndex: make(map[uint64]int),
		Frames:    make(map[uint64]pageRef),
		Pending:   make(map[uint64]int),
	}
}
