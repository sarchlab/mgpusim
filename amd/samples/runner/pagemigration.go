package runner

import (
	"flag"
	"fmt"
	"math/bits"

	"github.com/sarchlab/mgpusim/v5/amd/timing/pagemigration"
)

var pageMigrationFlag = flag.String("page-migration", "none",
	"Page migration between GPUs: none (off), observe (hooks and statistics "+
		"only, never migrates; the timing equals none), fault (migrate a page "+
		"to a GPU when a vector access of that GPU translates to it on "+
		"another GPU; the access waits), or counter (access counters: "+
		"migrate a unit to the GPU with the most remote accesses once one GPU "+
		"reaches -pm-threshold; asynchronous). Requires the serial engine.")
var pmUnitKBFlag = flag.Int("pm-unit-kb", 4,
	"Page migration: migration and counting unit in KB (a power of two, "+
		"e.g. 4 or 2048). A unit larger than the page moves as a group of "+
		"pages; the page size and the TLBs are unchanged.")
var pmSoftwareLatencyFlag = flag.Int("pm-sw-latency-ns", 5000,
	"Page migration: service time of one batch of faults or notifications "+
		"(driver work), per destination GPU.")
var pmBatchFlag = flag.Int("pm-batch", 256,
	"Page migration: most units serviced in one batch.")
var pmShootdownFlag = flag.Int("pm-shootdown-ns", 1000,
	"Page migration: TLB shootdown latency (accesses to the unit are held "+
		"from its start).")
var pmThresholdFlag = flag.Int("pm-threshold", 256,
	"Page migration (counter): remote accesses of one GPU to one unit that "+
		"trigger a migration.")
var pmCooldownFlag = flag.Int("pm-cooldown-ns", 5000,
	"Page migration: a unit does not migrate again within this time.")
var pmMaxMigrationsFlag = flag.Int("pm-max-migrations", 3,
	"Page migration: a unit that migrated this many times is pinned "+
		"(0 = unlimited).")
var pmChunkBytesFlag = flag.Int("pm-chunk-bytes", 4096,
	"Page migration: bytes per pull of a copy over the network.")
var pmPullWindowFlag = flag.Int("pm-pull-window", 16,
	"Page migration: pulls in flight per destination engine.")
var pmMemWindowFlag = flag.Int("pm-mem-window", 128,
	"Page migration: line accesses in flight from an engine to its L2 "+
		"(source reads) or its memory (destination writes).")
var pmIdealFlag = flag.Bool("pm-ideal", false,
	"Page migration upper bound: zero software latency and zero shootdown "+
		"latency (the TLB invalidation, the drain, the held accesses, and the "+
		"copy are still modeled).")
var pmCheckFlag = flag.Bool("pm-check", false,
	"Page migration: expensive invariant checks (every L2 of every GPU is "+
		"searched for lines of each new frame when it is allocated and when "+
		"it is mapped; a hit panics). The cheap checks are always on.")

// pageMigrationSpecFromFlags converts the page-migration flags.
func pageMigrationSpecFromFlags() pagemigration.Spec {
	policy, err := pagemigration.ParsePolicy(*pageMigrationFlag)
	if err != nil {
		panic(err)
	}

	kb := *pmUnitKBFlag
	if kb <= 0 || kb&(kb-1) != 0 {
		panic(fmt.Sprintf("-pm-unit-kb must be a power of two, got %d", kb))
	}

	s := pagemigration.DefaultSpec()
	s.Policy = policy
	s.Log2Unit = uint64(bits.TrailingZeros(uint(kb))) + 10
	s.SoftwareLatencyNS = uint64(*pmSoftwareLatencyFlag)
	s.BatchSize = *pmBatchFlag
	s.ShootdownLatencyNS = uint64(*pmShootdownFlag)
	s.Threshold = uint32(*pmThresholdFlag)
	s.CooldownNS = uint64(*pmCooldownFlag)
	s.MaxMigrations = *pmMaxMigrationsFlag
	s.ChunkBytes = uint64(*pmChunkBytesFlag)
	s.PullWindow = *pmPullWindowFlag
	s.MemWindow = *pmMemWindowFlag
	s.Check = *pmCheckFlag

	if *pmIdealFlag {
		s.SoftwareLatencyNS = 0
		s.ShootdownLatencyNS = 0
	}

	if s.Enabled() && *parallelFlag {
		panic("page migration requires the serial engine; remove -parallel")
	}

	return s
}
