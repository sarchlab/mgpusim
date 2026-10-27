package gputensor

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

func operatorSystem(t *testing.T) (*simulation.Simulation, *gpuruntime.Runtime, *driver.Comp, *GPUOperator) {
	t.Helper()
	s := simulation.MakeBuilder().Build()
	r := emusystem.MakeBuilder().WithSimulation(s).Build()
	o := NewGPUOperator(r, r.Init())
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			return s, r, d, o
		}
	}
	t.Fatal("Driver missing")
	return nil, nil, nil, nil
}
func runOperator(t *testing.T, s *simulation.Simulation, d *driver.Comp, o *GPUOperator) {
	t.Helper()
	d.TickLater()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if err := o.CompleteStep(); err != nil {
		t.Fatal(err)
	}
}
func readTensor(
	t *testing.T,
	s *simulation.Simulation,
	d *driver.Comp,
	o *GPUOperator,
	v tensor.Tensor,
) []float64 {
	t.Helper()
	id := o.EnqueueReadback(v)
	runOperator(t, s, d, o)
	if err := o.ReadTensor(id, v); err != nil {
		t.Fatal(err)
	}
	return v.Vector()
}

//nolint:gocognit // Keep each recovery/protocol scenario with its complete assertions.
func TestReadMutationReadAcrossAliasesAndRestore(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "restored"}[restore], func(t *testing.T) {
			s, r, d, o := operatorSystem(t)
			params := o.CreateWithData([]float64{1, 2, 3, 4}, []int{4}, "")
			view := o.Slice(params, 1, 3)
			if got := readTensor(t, s, d, o, params); !reflect.DeepEqual(got, []float64{1, 2, 3, 4}) {
				t.Fatal(got)
			}
			if restore {
				state := struct {
					Operator         OperatorState
					Parameters, View tensor.ID
				}{*o.OperatorState, o.Reference(params), o.Reference(view)}
				var buf bytes.Buffer
				if err := modeling.WriteCheckpoint(&buf, state, nil); err != nil {
					t.Fatal(err)
				}
				if err := modeling.ReadCheckpoint(&buf, &state, nil); err != nil {
					t.Fatal(err)
				}
				o = BindOperator(r, &state.Operator)
				params = o.Resolve(state.Parameters)
				view = o.Resolve(state.View)
			}
			o.Init(view, []float64{7, 9})
			func() {
				defer func() {
					if recover() == nil {
						t.Error("alias mutation exposed stale parameter cache")
					}
				}()
				_ = params.Vector()
			}()
			if got := readTensor(t, s, d, o, params); !reflect.DeepEqual(got, []float64{1, 7, 9, 4}) {
				t.Fatalf("alias output %v", got)
			}
			src := o.CreateWithData([]float64{4, 3, 2, 1}, []int{4}, "")
			o.Copy(params, src)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("D2D mutation exposed stale cache")
					}
				}()
				_ = params.Vector()
			}()
			if got := readTensor(t, s, d, o, params); !reflect.DeepEqual(got, []float64{4, 3, 2, 1}) {
				t.Fatalf("copy output %v", got)
			}
		})
	}
}
func TestDeferredFreeOwnsAllPagesAndAddressSpaces(t *testing.T) {
	s, r, d, o := operatorSystem(t)
	other := NewGPUOperator(r, r.Init())
	data := make([]float64, 1025)
	for i := range data {
		data[i] = float64(i % 17)
	}
	a := o.CreateWithData(data, []int{len(data)}, "")
	b := other.CreateWithData(data, []int{len(data)}, "")
	p := a.(*Tensor).Address
	if p != b.(*Tensor).Address {
		t.Fatal("test requires overlapping virtual addresses")
	}
	out := o.ReluForward(a)
	o.Free(a)
	if len(o.PendingFrees) != 1 || a.(*Tensor).Address != 0 {
		t.Fatal("logical release was not recorded")
	}
	for _, pid := range []uint32{1, 2} {
		for _, off := range []uint64{0, 4096} {
			if _, ok := d.Resources.PageTable.Find(vm.PID(pid), uint64(p)+off); !ok {
				t.Fatalf("early release PID %d offset %d", pid, off)
			}
		}
	}
	if err := o.CompleteStep(); err == nil {
		t.Fatal("released an undrained batch")
	}
	runOperator(t, s, d, o)
	if err := other.CompleteStep(); err != nil {
		t.Fatal(err)
	}
	for _, off := range []uint64{0, 4096} {
		if _, ok := d.Resources.PageTable.Find(1, uint64(p)+off); ok {
			t.Fatalf("missed page %x", uint64(p)+off)
		}
		if _, ok := d.Resources.PageTable.Find(2, uint64(p)+off); !ok {
			t.Fatal("freed another PID's allocation")
		}
	}
	if err := o.CompleteStep(); err != nil {
		t.Fatal("empty completion double-freed:", err)
	}
	if got := readTensor(t, s, d, o, out); !reflect.DeepEqual(got, data) {
		t.Fatal("kernel observed freed input")
	}
	if got := readTensor(t, s, d, other, b); !reflect.DeepEqual(got, data) {
		t.Fatal("other context corrupted")
	}
	if err := r.FreeMemory(o.Context, p); err == nil {
		t.Fatal("duplicate physical release accepted")
	}
}

func TestReadbackBeforeAliasedWriteCannotBecomeCurrentCache(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "restored"}[restore], func(t *testing.T) {
			s, rt, d, o := operatorSystem(t)
			v := o.CreateWithData([]float64{1, 2, 3, 4}, []int{4}, "")
			view := o.Slice(v, 1, 3)
			id := o.EnqueueReadback(v)
			o.Init(view, []float64{7, 9})
			if restore {
				state := *o.OperatorState
				saved := o.Reference(v)
				var buf bytes.Buffer
				if err := modeling.WriteCheckpoint(&buf, state, nil); err != nil {
					t.Fatal(err)
				}
				if err := modeling.ReadCheckpoint(&buf, &state, nil); err != nil {
					t.Fatal(err)
				}
				o = BindOperator(rt, &state)
				v = o.Resolve(saved)
			}
			runOperator(t, s, d, o)
			if err := o.ReadTensor(id, v); err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("old readback became current after a later aliased write")
					}
				}()
				v.Vector()
			}()
			if got := readTensor(t, s, d, o, v); !reflect.DeepEqual(got, []float64{1, 7, 9, 4}) {
				t.Fatal(got)
			}
			if len(o.ReadbackEpochs) != 0 {
				t.Fatal("consumed readback epochs retained")
			}
		})
	}
}

func TestRegistryRetiresTemporaryTensorsAcrossBatches(t *testing.T) {
	s, _, d, o := operatorSystem(t)
	o.SetVerification(true)
	params := o.CreateWithData([]float64{1, 2, 3, 4}, []int{4}, "")
	view := o.Slice(params, 1, 3)
	for batch := 0; batch < 12; batch++ {
		tmp := o.Clone(params)
		out := o.ReluForward(tmp)
		o.Free(tmp)
		o.Free(out)
		if len(o.Tensors) != 2 || len(o.tensors) != 2 {
			t.Fatalf("batch%d retained dead tensors: %d/%d", batch, len(o.Tensors), len(o.tensors))
		}
		runOperator(t, s, d, o)
		if len(o.PendingFrees) != 0 || len(o.Checks) != 0 {
			t.Fatal("batch did not consume releases/checks")
		}
	}
	o.Init(view, []float64{7, 9})
	if got := readTensor(t, s, d, o, params); !reflect.DeepEqual(got, []float64{1, 7, 9, 4}) {
		t.Fatal(got)
	}
}
