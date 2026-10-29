package runner

import (
	"flag"
	"fmt"
	"math/bits"

	"github.com/sarchlab/mgpusim/v5/amd/timing/pagemigration"
)

var pageMigrationFlag = flag.String("page-migration", "none",
	"Page migration between GPUs: none (off), observe (count the remote "+
		"accesses only, never migrate; the timing equals none), fault "+
		"(migrate a page to a GPU when a vector access of that GPU walks to it "+
		"on another GPU; the access waits), or counter (access counters: "+
		"migrate a unit to a GPU once its remote accesses to one page reach "+
		"-pm-threshold; asynchronous).")
var pmUnitKBFlag = flag.Int("pm-unit-kb", 4,
	"Page migration: migration unit in KB (a power of two, e.g. 4 or 2048). "+
		"A unit larger than the page moves as a group of pages; the page "+
		"size and the TLBs are unchanged.")
var pmSoftwareLatencyFlag = flag.Int("pm-sw-latency-ns", 5000,
	"Page migration: service time of one batch of faults or notifications "+
		"(driver work), per destination GPU.")
var pmBatchFlag = flag.Int("pm-batch", 256,
	"Page migration: most units serviced in one batch.")
var pmShootdownFlag = flag.Int("pm-shootdown-ns", 1000,
	"Page migration: least TLB shootdown time (the TLBs of every GPU are "+
		"also paused, invalidated, and enabled with messages).")
var pmThresholdFlag = flag.Int("pm-threshold", 256,
	"Page migration (counter): remote accesses of one GPU to one page that "+
		"trigger a migration.")
var pmCooldownFlag = flag.Int("pm-cooldown-ns", 5000,
	"Page migration: a unit does not migrate again within this time; a "+
		"fault on it is mapped remotely until then.")
var pmMaxMigrationsFlag = flag.Int("pm-max-migrations", 3,
	"Page migration: a unit that migrated this many times is pinned "+
		"(0 = unlimited).")
var pmCopyBufferFlag = flag.Int("pm-copy-buffer-bytes", 64<<10,
	"Page migration: bytes of a copy that the data mover keeps in flight.")
var pmIdealFlag = flag.Bool("pm-ideal", false,
	"Page migration upper bound: zero software latency and zero shootdown "+
		"latency (the hold, the copy, the TLB invalidation, and the held "+
		"accesses are still modeled).")

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
	s.CopyBufferBytes = uint64(*pmCopyBufferFlag)

	if *pmIdealFlag {
		s.SoftwareLatencyNS = 0
		s.ShootdownLatencyNS = 0
	}

	return s
}
