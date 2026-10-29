package pagemigration

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// Metric is one statistic of page migration.
type Metric struct {
	Location string
	What     string
	Value    float64
	Unit     string
}

type withMiddlewares interface {
	Name() string
	Middlewares() []modeling.Middleware
}

// DriverMiddlewareOf returns the page-migration middleware of component c,
// or nil if c has none.
func DriverMiddlewareOf(c any) *DriverMiddleware {
	comp, ok := c.(withMiddlewares)
	if !ok {
		return nil
	}

	for _, mw := range comp.Middlewares() {
		if dmw, ok := mw.(*DriverMiddleware); ok {
			return dmw
		}
	}

	return nil
}

// Metrics returns the configuration and the statistics of page migration,
// read from the State of the components. It returns nothing if no
// component runs a page-migration driver middleware.
func Metrics[C any](components []C) []Metric {
	var (
		dmw      *DriverMiddleware
		location string
		counters metricCounters
	)

	for _, c := range components {
		if m := DriverMiddlewareOf(c); m != nil {
			dmw = m
			location = any(c).(withMiddlewares).Name()
		}

		counters.add(c)
	}

	if dmw == nil {
		return nil
	}

	var metrics []Metric

	add := func(what string, value float64, unit string) {
		metrics = append(metrics, Metric{
			Location: location, What: what, Value: value, Unit: unit,
		})
	}

	addDriverMetrics(dmw, add)
	counters.report(add)

	for dev, n := range counters.readsByOwner {
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

func addDriverMetrics(dmw *DriverMiddleware, add addMetricFunc) {
	spec := dmw.Spec()
	state := dmw.State()
	s := state.Stats

	add("pm_policy_code", float64(spec.Policy), "")
	add("pm_unit_bytes", float64(uint64(1)<<spec.Log2Unit), "byte")

	for _, c := range []struct {
		what  string
		value uint64
	}{
		{"pm_faults", s.Faults},
		{"pm_counter_notifications", s.CounterNotifications},
		{"pm_counter_triggers", s.CounterTriggers},
		{"pm_batches", s.Batches},
		{"pm_services", s.Services},
		{"pm_migrations", s.Migrations},
		{"pm_noop_services", s.NoopServices},
		{"pm_remote_maps", s.RemoteMaps},
		{"pm_pages_migrated", s.PagesMigrated},
		{"pm_bytes_migrated", s.BytesMigrated},
		{"pm_pingpong", s.PingPong},
		{"pm_pinned_units", s.Pinned},
		{"pm_shootdowns", s.Shootdowns},
		{"pm_hold_reqs", s.HoldReqs},
		{"pm_units_touched", uint64(len(state.Units))},
	} {
		add(c.what, float64(c.value), "count")
	}

	hist := migrationsPerUnit(state)
	for k := 1; k < len(hist); k++ {
		if hist[k] > 0 {
			add(fmt.Sprintf("pm_units_migrated_%dx", k), float64(hist[k]), "count")
		}
	}

	if s.Migrations == 0 {
		return
	}

	avg := func(sum uint64) float64 {
		return float64(sum) * 1e-12 / float64(s.Migrations)
	}

	add("pm_avg_migration_time", avg(s.MigrationTimeSum), "second")
	add("pm_avg_blocked_time", avg(s.BlockedTimeSum), "second")
	add("pm_avg_copy_time", avg(s.CopyTimeSum), "second")
	add("pm_avg_shootdown_time", avg(s.ShootdownTimeSum), "second")
}

// migrationsPerUnit returns, at index k, the number of units that migrated k
// times.
func migrationsPerUnit(state *DriverState) []int {
	var hist []int

	for _, u := range state.Units {
		for len(hist) <= u.Migrations {
			hist = append(hist, 0)
		}

		hist[u.Migrations]++
	}

	return hist
}

// metricCounters sums the page-migration counters of the address
// translators, the MMUs, and the RDMA engines.
type metricCounters struct {
	heldAccesses   uint64
	atFaults       uint64
	mmuFaults      uint64
	remoteAccesses uint64
	remoteReads    uint64
	readsByOwner   []uint64
}

func (c *metricCounters) add(comp any) {
	switch comp := comp.(type) {
	case *addresstranslator.Comp:
		c.heldAccesses += comp.State.NumHeldReqs
		c.atFaults += comp.State.NumFaults
	case *mmu.Comp:
		c.mmuFaults += comp.State.NumFaults
	case *rdma.Comp:
		counter := &comp.State.AccessCounter
		c.remoteAccesses += counter.RemoteAccesses
		c.remoteReads += counter.RemoteReads

		for owner, n := range counter.RemoteReadsByOwner {
			for uint64(len(c.readsByOwner)) <= owner {
				c.readsByOwner = append(c.readsByOwner, 0)
			}

			c.readsByOwner[owner] += n
		}
	}
}

func (c *metricCounters) report(add addMetricFunc) {
	add("pm_held_accesses", float64(c.heldAccesses), "count")
	add("pm_at_faulted_accesses", float64(c.atFaults), "count")
	add("pm_mmu_faults", float64(c.mmuFaults), "count")
	add("pm_remote_accesses", float64(c.remoteAccesses), "count")
	add("pm_remote_reads", float64(c.remoteReads), "count")

	var total, best, owner uint64

	for d, n := range c.readsByOwner {
		total += n
		if n > best {
			best, owner = n, uint64(d)
		}
	}

	share := 0.0
	if total > 0 {
		share = float64(best) / float64(total)
	}

	add("pm_hot_owner_read_share", share, "")
	add("pm_hot_owner_device", float64(owner), "")
}
