package pagemigration

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// Policy selects what triggers a migration.
type Policy int

// Policies.
const (
	// PolicyNone disables page migration. Nothing is built.
	PolicyNone Policy = iota

	// PolicyObserve counts the remote accesses for the statistics but never
	// migrates. It must give the same timing as PolicyNone.
	PolicyObserve

	// PolicyOnFault migrates a page to a GPU when a vector access of that GPU
	// translates to it on another GPU. The translation faults at the MMU and
	// the access waits.
	PolicyOnFault

	// PolicyAccessCounter counts the remote accesses of each GPU to each page
	// and migrates the unit of a page, asynchronously, to a GPU whose count
	// reaches the threshold.
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

	// Log2Unit is the migration granularity: 12 for 4 KB, 21 for 2 MB. It
	// must not be smaller than the page size. A unit larger than a page
	// moves as a group of pages; the page size and the TLBs are not changed.
	Log2Unit uint64 `json:"log2_unit"`

	// SoftwareLatencyNS is the service time of one batch of faults or
	// notifications (driver work), BatchSize the most units in a batch.
	SoftwareLatencyNS uint64 `json:"software_latency_ns"`
	BatchSize         int    `json:"batch_size"`

	// ShootdownLatencyNS is the least time from the start of a TLB shootdown
	// until it completes. The shootdown also waits until every TLB of every
	// GPU has invalidated the pages.
	ShootdownLatencyNS uint64 `json:"shootdown_latency_ns"`

	// Threshold is the access-counter threshold: remote accesses of one GPU
	// to one page, counted by the RDMA engine of the GPU.
	Threshold uint32 `json:"threshold"`

	// A unit does not migrate again within CooldownNS after its last
	// migration; a fault on it in that time maps it remotely until the
	// cooldown ends. A unit that migrated MaxMigrations times is pinned:
	// it is mapped remotely for good (0 = unlimited).
	CooldownNS    uint64 `json:"cooldown_ns"`
	MaxMigrations int    `json:"max_migrations"`

	// The data mover of each GPU copies a page with line accesses of
	// LineBytes and keeps up to CopyBufferBytes of a copy in flight.
	LineBytes       uint64 `json:"line_bytes"`
	CopyBufferBytes uint64 `json:"copy_buffer_bytes"`

	// New frames for migrated pages are taken, in order and never reused,
	// from [FrameRegionOffset, FrameRegionOffset+FrameRegionSize) of each
	// GPU's physical memory. The driver allocates from the bottom of the
	// memory, so the region must be above everything a workload allocates.
	FrameRegionOffset uint64 `json:"frame_region_offset"`
	FrameRegionSize   uint64 `json:"frame_region_size"`

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
		LineBytes:          64,
		CopyBufferBytes:    64 << 10,
		FrameRegionOffset:  3 << 30,
		FrameRegionSize:    1 << 30,
		Log2PageSize:       12,
		GPUMemSize:         4 << 30,
	}
}

// Enabled reports whether page migration is built at all.
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

	if s.Log2Unit < s.Log2PageSize {
		return fmt.Errorf("migration unit 2^%d is smaller than the page 2^%d",
			s.Log2Unit, s.Log2PageSize)
	}

	pageSize := uint64(1) << s.Log2PageSize
	if s.LineBytes == 0 || pageSize%s.LineBytes != 0 ||
		s.CopyBufferBytes < s.LineBytes {
		return fmt.Errorf("line %d must divide the page %d and fit in the "+
			"copy buffer %d", s.LineBytes, pageSize, s.CopyBufferBytes)
	}

	if s.BatchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}

	if s.Policy == PolicyAccessCounter && s.Threshold == 0 {
		return fmt.Errorf("access-counter threshold must be positive")
	}

	if s.FrameRegionSize < pageSize ||
		s.FrameRegionOffset+s.FrameRegionSize > s.GPUMemSize {
		return fmt.Errorf("frame region [%#x, %#x) is not inside the %#x-byte "+
			"memory of a GPU", s.FrameRegionOffset,
			s.FrameRegionOffset+s.FrameRegionSize, s.GPUMemSize)
	}

	return nil
}

func nsToPS(ns uint64) timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(ns * 1000)
}
