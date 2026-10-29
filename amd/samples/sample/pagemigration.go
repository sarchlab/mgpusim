package sample

import (
	"flag"
	"fmt"
	"math/bits"

	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Page-migration flags (sarchlab/akita#589). The defaults follow the NVIDIA
// UVM driver.
var (
	pageMigrationFlag = flag.String("page-migration", "none",
		"Page migration between GPUs: none, or fault (the first access of a "+
			"GPU to a unified page resident on another GPU faults, waits, and "+
			"the page moves to the faulting GPU).")
	pmUnitKBFlag = flag.Int("pm-unit-kb", 4,
		"Page migration: unit in KB, a power of two from 4 to 2048.")
	pmSoftwareLatencyFlag = flag.Uint64("pm-sw-latency-ns", 5000,
		"Page migration: time the driver collects a batch of faults.")
	pmBatchFlag = flag.Int("pm-batch", 256,
		"Page migration: most faults in a batch (uvm_perf_fault_batch_count).")
	pmThrashingLapseFlag = flag.Uint64("pm-thrashing-lapse-ns", 500000,
		"Page migration: a migration within this time of the previous one "+
			"is a thrashing event (uvm_perf_thrashing_lapse_usec).")
	pmThrashingThresholdFlag = flag.Int("pm-thrashing-threshold", 3,
		"Page migration: consecutive thrashing events that pin a unit "+
			"(uvm_perf_thrashing_threshold).")
	pmPinFlag = flag.Uint64("pm-pin-ns", 150000000,
		"Page migration: how long a thrashing unit stays pinned "+
			"(uvm_perf_thrashing_pin).")
)

// pageMigrationSpecFromFlags converts the page-migration flags.
func pageMigrationSpecFromFlags() driver.PageMigrationSpec {
	kb := *pmUnitKBFlag
	if kb < 4 || kb > 2048 || kb&(kb-1) != 0 {
		panic(fmt.Sprintf("-pm-unit-kb must be a power of two from 4 to 2048, got %d", kb))
	}

	s := driver.DefaultPageMigrationSpec()
	s.Policy = *pageMigrationFlag
	s.Log2Unit = uint64(bits.TrailingZeros(uint(kb))) + 10
	s.SoftwareLatencyNS = *pmSoftwareLatencyFlag
	s.BatchSize = *pmBatchFlag
	s.ThrashingLapseNS = *pmThrashingLapseFlag
	s.ThrashingThreshold = *pmThrashingThresholdFlag
	s.PinNS = *pmPinFlag

	return s
}
