package driver_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type kernelTimes struct {
	tasks.NopObserver
	intervals [][2]timing.VTimeInPicoSec
}

func (k *kernelTimes) TaskEnded(t tasks.Task) {
	if t.What == "*driver.LaunchKernelCommand" {
		k.intervals = append(
			k.intervals,
			[2]timing.VTimeInPicoSec{t.StartTime, t.EndTime},
		)
	}
}
func eventSystem(t *testing.T) (*sim.Simulation, *driver.Runtime, interface{ TickLater() }) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			return s, rt, c.(interface{ TickLater() })
		}
	}
	t.Fatal("missing Driver")
	return nil, nil, nil
}
func eventGroup(rt *driver.Runtime, q driver.CommandQueue) []driver.Event {
	return []driver.Event{rt.EnqueueRecordEvent(q), rt.EnqueueRecordEvent(q), rt.EnqueueRecordEvent(q)}
}
func checkEventGroup(t *testing.T, rt *driver.Runtime, events []driver.Event, want timing.VTimeInPicoSec) {
	t.Helper()
	for _, event := range events {
		got, err := rt.EventTime(event)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("event time %d, want %d", got, want)
		}
	}
}
func TestMarkerOnlyQueueHasOneTriggerTime(t *testing.T) {
	s, rt, d := eventSystem(t)
	q := rt.CreateCommandQueue(rt.Init())
	events := eventGroup(rt, q)
	d.TickLater()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	checkEventGroup(t, rt, events, s.Engine().CurrentTime())
	if rt.Unfinished()[0] != 0 {
		t.Fatal("markers did not complete")
	}
}
func submitFIRPair(t *testing.T, rt *driver.Runtime, markers bool) ([]driver.CommandID, [][]driver.Event) {
	t.Helper()
	ctx := rt.Init()
	q := rt.CreateCommandQueue(ctx)
	input, output, filter, history := rt.AllocateMemory(
		ctx,
		256,
	), rt.AllocateMemory(
		ctx,
		256,
	), rt.AllocateMemory(
		ctx,
		16,
	), rt.AllocateMemory(
		ctx,
		16,
	)
	data := make([]float32, 64)
	for i := range data {
		data[i] = float32(i)
	}
	rt.EnqueueMemCopyH2D(
		q,
		input,
		data,
	)
	rt.EnqueueMemCopyH2D(
		q,
		filter,
		[]float32{0, 1, 2, 3},
	)
	rt.EnqueueMemCopyH2D(
		q,
		history,
		[]float32{0, 0, 0, 0},
	)
	code, err := os.ReadFile("../benchmarks/heteromark/fir/kernels.hsaco")
	if err != nil {
		t.Fatal(err)
	}
	co := insts.LoadKernelCodeObjectFromBytes(code, "FIR")
	var events [][]driver.Event
	var reads []driver.CommandID
	if markers {
		events = append(events, eventGroup(rt, q))
	}
	for i := 0; i < 2; i++ {
		args := fir.GCN3KernelArgs{Input: input, Output: output, Filter: filter, History: history, NumTaps: 4}
		rt.EnqueueLaunchKernel(q, co, [3]uint32{64, 1, 1}, [3]uint16{64, 1, 1}, &args)
		if markers {
			events = append(events, eventGroup(rt, q))
		}
		reads = append(reads, rt.EnqueueMemCopyD2H(q, output, 256))
	}
	return reads, events
}
func verifyFIRReads(t *testing.T, rt *driver.Runtime, ids []driver.CommandID) {
	t.Helper()
	for _, id := range ids {
		values := make([]float32, 64)
		if err := rt.ReadResult(id, values); err != nil {
			t.Fatal(err)
		}
		for i, v := range values {
			var want float32
			for j := 0; j < 4; j++ {
				if i >= j {
					want += float32(i-j) * float32(j)
				}
			}
			if v != want {
				t.Fatalf("FIR[%d]=%g, want %g", i, v, want)
			}
		}
	}
}
func TestMarkersDoNotChangeKernelOrSimulationTiming(t *testing.T) {
	var baselineEnd timing.VTimeInPicoSec
	var baseline [][2]timing.VTimeInPicoSec
	for _, marked := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "markers"}[marked], func(t *testing.T) {
			s, rt, d := eventSystem(t)
			times := &kernelTimes{}
			for _, c := range s.Components() {
				if c.Name() == "Driver" {
					s.TaskTracker().Subscribe(times, c)
				}
			}
			ids, events := submitFIRPair(t, rt, marked)
			d.TickLater()
			if err := s.Run(); err != nil {
				t.Fatal(err)
			}
			verifyFIRReads(t, rt, ids)
			if len(times.intervals) != 2 {
				t.Fatalf("kernel intervals %v", times.intervals)
			}
			if !marked {
				baselineEnd = s.Engine().CurrentTime()
				baseline = times.intervals
				return
			}
			if !reflect.DeepEqual(times.intervals, baseline) || s.Engine().CurrentTime() != baselineEnd {
				t.Fatalf(
					"markers changed timing: kernels %v/%v, finish %d/%d",
					times.intervals,
					baseline,
					s.Engine().CurrentTime(),
					baselineEnd,
				)
			}
			for i := 0; i < 2; i++ {
				checkEventGroup(t, rt, events[i+1], times.intervals[i][1])
			}
			first, err := rt.EventTime(events[0][0])
			if err != nil {
				t.Fatal(err)
			}
			checkEventGroup(t, rt, events[0], first)
		})
	}
}
