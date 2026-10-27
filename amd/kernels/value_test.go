package kernels

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func testLaunch() KernelLaunchInfo {
	return KernelLaunchInfo{
		CodeObject: &insts.KernelCodeObject{
			KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{WFSgprCount: 8},
			Data:                 []byte{1, 2, 3, 4},
			Version:              insts.CodeObjectV5,
		},
		Packet: &HsaKernelDispatchPacket{
			WorkgroupSizeX: 8,
			WorkgroupSizeY: 8,
			WorkgroupSizeZ: 2,
			GridSizeX:      20,
			GridSizeY:      12,
			GridSizeZ:      3,
		},
		PacketAddr: 4096,
	}
}

func TestWorkGroupValueRestoresLinks(t *testing.T) {
	builder := NewGridBuilder(sim.MakeBuilder().Build())
	builder.SetKernel(testLaunch())
	for original := builder.NextWG(); original != nil; original = builder.NextWG() {
		original.Wavefronts[0].IssueTime = 100
		var data bytes.Buffer
		if err := modeling.WriteCheckpoint(&data, struct{}{}, WorkGroupSnapshot(original), nil); err != nil {
			t.Fatal(err)
		}
		var value WorkGroupValue
		if err := modeling.ReadCheckpoint(&data, struct{}{}, &value, nil); err != nil {
			t.Fatal(err)
		}
		restored, err := value.Restore()
		if err != nil {
			t.Fatal(err)
		}
		originalCode := original.CodeObject.Data
		original.CodeObject.Data = nil
		if !reflect.DeepEqual(original, restored) {
			t.Fatalf("work-group %d changed", original.UID)
		}
		for _, wf := range restored.Wavefronts {
			if wf.WG != restored || wf.Packet != restored.Packet || wf.CodeObject != restored.CodeObject {
				t.Fatal("restored wavefront has detached parent")
			}
			for _, wi := range wf.WorkItems {
				if wi.WG != restored {
					t.Fatal("restored work-item has detached parent")
				}
			}
		}
		original.CodeObject.Data = originalCode
		if len(restored.CodeObject.Data) != 0 {
			t.Fatal("dispatch snapshot duplicated executable bytes")
		}
		restored.CodeObject.WFSgprCount = 99
		if original.CodeObject.WFSgprCount != 8 {
			t.Fatal("metadata aliases caller")
		}
	}
}

func TestGridSelectionAndCursorRestore(t *testing.T) {
	launch := testLaunch()
	launch.Selection = WGSelection{Partitioned: true, First: 2, End: 10}
	ids := sim.MakeBuilder().Build()
	builder := NewGridBuilder(ids)
	builder.SetKernel(launch)
	if builder.NumWG() != 8 {
		t.Fatal("partition count changed")
	}
	first := builder.NextWG()
	if first.IDX != 2 || first.IDY != 0 || first.IDZ != 0 {
		t.Fatal("partition no longer uses X-fastest order")
	}
	var data bytes.Buffer
	if err := modeling.WriteCheckpoint(&data, struct{}{}, builder.Snapshot(), nil); err != nil {
		t.Fatal(err)
	}
	var snapshot GridBuilderState
	if err := modeling.ReadCheckpoint(&data, struct{}{}, &snapshot, nil); err != nil {
		t.Fatal(err)
	}
	restored := NewGridBuilder(ids)
	restored.Restore(snapshot)
	for count := 1; ; count++ {
		want, got := builder.NextWG(), restored.NextWG()
		if want == nil || got == nil {
			if want != nil || got != nil || count != 8 {
				t.Fatal("restored partition ended at wrong point")
			}
			break
		}
		if want.IDX != got.IDX || want.IDY != got.IDY || want.IDZ != got.IDZ || want.CurrSizeX != got.CurrSizeX ||
			want.CurrSizeY != got.CurrSizeY ||
			want.CurrSizeZ != got.CurrSizeZ {
			t.Fatal("restored cursor changed work-group geometry")
		}
	}
}

func TestDispatchSnapshotSizeIndependentOfExecutable(t *testing.T) {
	measure := func(size int) int {
		launch := testLaunch()
		launch.CodeObject.Data = make([]byte, size)
		b := NewGridBuilder(sim.MakeBuilder().Build())
		b.SetKernel(launch)
		var data bytes.Buffer
		if err := modeling.WriteCheckpoint(&data, struct{}{}, WorkGroupSnapshot(b.NextWG()), nil); err != nil {
			t.Fatal(err)
		}
		return data.Len()
	}
	small, large := measure(4), measure(1<<20)
	if small != large {
		t.Fatalf("work-group checkpoint embeds executable: %d -> %d", small, large)
	}
	t.Logf("work-group snapshot: %d bytes with both 4-byte and 1MiB executable", small)
}
