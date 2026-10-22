package pagemigration

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// Policy selects what triggers a migration.
type Policy int

// Policies.
const (
	// PolicyNone disables page migration. Nothing is built or hooked.
	PolicyNone Policy = iota

	// PolicyObserve installs the hooks and collects the statistics but never
	// migrates. It must give the same timing as PolicyNone.
	PolicyObserve

	// PolicyOnFault migrates a page to a GPU when an access of that GPU
	// translates to a page on another GPU. The access waits.
	PolicyOnFault

	// PolicyAccessCounter counts the remote accesses of each GPU to each unit
	// and migrates a unit, asynchronously, to the GPU with the most accesses
	// once one GPU reaches the threshold.
	PolicyAccessCounter
)

// ParsePolicy converts a flag value to a Policy.
func ParsePolicy(s string) (Policy, error) {
	switch s {
	case "", "none", "off":
		return PolicyNone, nil
	case "observe":
		return PolicyObserve, nil
	case "fault", "on-fault":
		return PolicyOnFault, nil
	case "counter", "access-counter":
		return PolicyAccessCounter, nil
	}

	return PolicyNone, fmt.Errorf("unknown page-migration policy %q", s)
}

func (p Policy) String() string {
	switch p {
	case PolicyObserve:
		return "observe"
	case PolicyOnFault:
		return "fault"
	case PolicyAccessCounter:
		return "counter"
	default:
		return "none"
	}
}

// Spec holds every parameter of the migration model. Latencies are in
// nanoseconds.
type Spec struct {
	Freq   timing.Freq `json:"freq"`
	Policy Policy      `json:"policy"`

	// Log2Unit is the migration (and access-counting) granularity: 12 for
	// 4 KB, 21 for 2 MB. It must not be smaller than the page size. A unit
	// larger than a page moves as a group of pages; the page size and the
	// TLBs are not changed.
	Log2Unit uint64 `json:"log2_unit"`

	// SoftwareLatencyNS is the service time of one batch of faults or
	// notifications (driver work), BatchSize the most units in a batch.
	SoftwareLatencyNS uint64 `json:"software_latency_ns"`
	BatchSize         int    `json:"batch_size"`

	// ShootdownLatencyNS is the time from the start of a TLB shootdown until
	// all GPUs acknowledged it. The shootdown also waits for the TLB
	// invalidation messages themselves.
	ShootdownLatencyNS uint64 `json:"shootdown_latency_ns"`

	// Threshold is the access-counter threshold: remote accesses of one GPU
	// to one unit.
	Threshold uint32 `json:"threshold"`

	// A unit does not migrate again within CooldownNS after its last
	// migration. A unit that migrated MaxMigrations times is pinned
	// (0 = unlimited).
	CooldownNS    uint64 `json:"cooldown_ns"`
	MaxMigrations int    `json:"max_migrations"`

	// Copy engines: bytes per pull over the network, pulls in flight per
	// destination engine, line accesses in flight from an engine to its L2
	// or DRAM, and the line size.
	ChunkBytes uint64 `json:"chunk_bytes"`
	PullWindow int    `json:"pull_window"`
	MemWindow  int    `json:"mem_window"`
	LineBytes  uint64 `json:"line_bytes"`

	// New frames for migrated pages are taken, in order and never reused,
	// from [FrameRegionOffset, FrameRegionOffset+FrameRegionSize) of each
	// GPU's physical memory. The driver allocates from the bottom of the
	// memory, so the region must be above everything a workload allocates.
	FrameRegionOffset uint64 `json:"frame_region_offset"`
	FrameRegionSize   uint64 `json:"frame_region_size"`

	// Check enables the expensive invariant checks: every L2 of every GPU is
	// searched for lines of each new frame when it is allocated and when the
	// page is remapped.
	Check bool `json:"check"`

	// Log2PageSize is the page size of the page table and the TLBs.
	// GPUMemSize is the size of the physical memory of each device; device d
	// (the CPU is device 0) owns [d*GPUMemSize, (d+1)*GPUMemSize). The
	// platform builder sets both.
	Log2PageSize uint64 `json:"log2_page_size"`
	GPUMemSize   uint64 `json:"gpu_mem_size"`
}

// DefaultSpec returns the default parameters. The access-counter threshold,
// batch size, and migration cap follow the defaults of the NVIDIA UVM driver
// (uvm_perf_access_counter_threshold = 256, a batch of 256 faults or
// notifications, uvm_perf_thrashing_threshold = 3). The software latency is
// at the low end of reported GPU far-fault service times, which favors
// migration.
func DefaultSpec() Spec {
	return Spec{
		Freq:               1 * timing.GHz,
		Policy:             PolicyNone,
		Log2Unit:           12,
		SoftwareLatencyNS:  5000,
		BatchSize:          256,
		ShootdownLatencyNS: 1000,
		Threshold:          256,
		CooldownNS:         5000,
		MaxMigrations:      3,
		ChunkBytes:         4096,
		PullWindow:         16,
		MemWindow:          128,
		LineBytes:          64,
		FrameRegionOffset:  3 << 30,
		FrameRegionSize:    1 << 30,
		Log2PageSize:       12,
		GPUMemSize:         4 << 30,
	}
}

// Enabled reports whether the hooks are installed.
func (s Spec) Enabled() bool { return s.Policy != PolicyNone }

// Migrates reports whether the policy moves pages.
func (s Spec) Migrates() bool {
	return s.Policy == PolicyOnFault || s.Policy == PolicyAccessCounter
}

// Validate checks the parameters against the page size and the size of the
// physical memory of each GPU.
func (s Spec) Validate() error {
	if !s.Enabled() {
		return nil
	}

	log2PageSize, gpuMemSize := s.Log2PageSize, s.GPUMemSize

	if s.Log2Unit < log2PageSize {
		return fmt.Errorf("migration unit 2^%d is smaller than the page 2^%d",
			s.Log2Unit, log2PageSize)
	}

	pageSize := uint64(1) << log2PageSize
	if s.LineBytes == 0 || s.ChunkBytes == 0 || pageSize%s.ChunkBytes != 0 ||
		s.ChunkBytes%s.LineBytes != 0 {
		return fmt.Errorf("chunk %d must divide the page %d and be a multiple "+
			"of the line %d", s.ChunkBytes, pageSize, s.LineBytes)
	}

	if s.BatchSize <= 0 || s.PullWindow <= 0 || s.MemWindow <= 0 {
		return fmt.Errorf("batch size and windows must be positive")
	}

	if s.Policy == PolicyAccessCounter && s.Threshold == 0 {
		return fmt.Errorf("access-counter threshold must be positive")
	}

	if s.FrameRegionSize < pageSize ||
		s.FrameRegionOffset+s.FrameRegionSize > gpuMemSize {
		return fmt.Errorf("frame region [%#x, %#x) is not inside the %#x-byte "+
			"memory of a GPU", s.FrameRegionOffset,
			s.FrameRegionOffset+s.FrameRegionSize, gpuMemSize)
	}

	return nil
}

func nsToPS(ns uint64) timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(ns * 1000)
}
