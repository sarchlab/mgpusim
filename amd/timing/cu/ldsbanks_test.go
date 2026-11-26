package cu

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func TestLDSBankPhases(t *testing.T) {
	const full = ^uint64(0)
	b32 := insts.CDNA3LDSAccess{Bytes: 4, Count: 1}
	for _, tc := range []struct {
		stride  uint32
		replays int
	}{{4, 0}, {8, 2}, {16, 6}, {32, 14}, {64, 30}, {128, 62}} {
		var bases [64]uint32
		for i := range bases {
			bases[i] = uint32(i) * tc.stride
		}
		rounds, replays := ldsBankRounds(b32, full, &bases)
		if replays != tc.replays || rounds != tc.replays+2 {
			t.Fatalf("stride %d: got %d rounds, %d replays", tc.stride, rounds, replays)
		}
	}
	// Lanes 0 and 20 share one b128 read phase and use two write phases.
	bases := [64]uint32{20: 128, 32: 128}
	for _, tc := range []struct {
		name            string
		access          insts.CDNA3LDSAccess
		exec            uint64
		rounds, replays int
	}{
		{"broadcast", b32, 7, 1, 0},
		{"distinct phases", b32, 1 | 1<<32, 2, 0},
		{"inactive", b32, 0, 0, 0},
		{"b128 load", insts.CDNA3LDSAccess{Bytes: 16, Count: 1}, 1 | 1<<20, 2, 1},
		{"b128 store", insts.CDNA3LDSAccess{Bytes: 16, Count: 1, Write: true}, 1 | 1<<20, 2, 0},
		{"b32 store", insts.CDNA3LDSAccess{Bytes: 4, Count: 1, Write: true}, 7, 3, 2},
		{"paired b64", insts.CDNA3LDSAccess{Bytes: 8, Count: 2, Offsets: [2]uint32{8, 16}}, 1 | 1<<20, 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rounds, replays := ldsBankRounds(tc.access, tc.exec, &bases); rounds != tc.rounds || replays != tc.replays {
				t.Fatalf("got (%d,%d), want (%d,%d)", rounds, replays, tc.rounds, tc.replays)
			}
		})
	}
}

func TestLDSWritebackAfterCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   []byte
		base  uint64
		vmcnt int
	}{
		// ds_read_b64 v[4:5], v0 offset:8
		{"DS", []byte{8, 0, 0xec, 0xd8, 0, 0, 0, 4}, 0, 0},
		// flat_load_dwordx2 v[4:5], v[0:1] offset:8
		{"shared FLAT", []byte{8, 0, 0x54, 0xdc, 0, 0, 0, 4}, insts.CDNA3SharedApertureBase, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := DefaultSpec()
			spec.IsCDNA3 = true
			f := newVMUFixture(t, spec)
			w := checkpointGroup(f.c).waves()[0]
			i := lifecycleInstruction(t, f.c, tc.raw)
			w.setTestInst(i)
			w.execution().State = wavefront.WfRunning
			f.c.beginInstruction(w, i.ID)
			w.SetEXEC(1<<2 | 1<<51)
			w.group().state().LDS = make([]byte, 32)
			copy(w.group().state().LDS[8:], []byte{7, 6, 5, 4, 3, 2, 1, 9})
			w.WriteOperand(i.Addr, 2, tc.base)
			w.WriteOperand(i.Addr, 51, tc.base)
			f.c.LDSUnit.AcceptWave(w)
			if !f.c.LDSUnit.Run() || w.execution().OutstandingScalarMemAccess != 1 ||
				w.execution().OutstandingVectorMemAccess != tc.vmcnt || w.execution().State != wavefront.WfReady {
				t.Fatal("LDS issue did not free the wave with the expected wait counters")
			}
			if w.ReadOperand(i.Dst, 2) != 0 {
				t.Fatal("load result visible at issue")
			}
			// A later instruction can change EXEC, VGPR sources and wave PC.
			// Neither that mask nor later LDS writes may change the saved result.
			w.SetEXEC(1)
			w.SetInstID(0)
			clear(w.group().state().LDS)
			var component, engine bytes.Buffer
			if err := f.c.comp.SaveCheckpoint(&component); err != nil {
				t.Fatal(err)
			}
			if err := f.engine.SaveCheckpoint(&engine); err != nil {
				t.Fatal(err)
			}
			r := newVMUFixture(t, spec)
			if err := r.c.comp.LoadCheckpoint(&component); err != nil {
				t.Fatal(err)
			}
			if err := r.engine.LoadCheckpoint(&engine); err != nil {
				t.Fatal(err)
			}
			assertLifecycleHolders(t, r.c)
			r.run()
			checkLDSResult(t, r.c.wave(w.handle), i)
		})
	}
}

func checkLDSResult(t *testing.T, w *Wavefront, i *testInstruction) {
	t.Helper()
	const data = 0x0901020304050607
	if w.ReadOperand(i.Dst, 2) != data || w.ReadOperand(i.Dst, 51) != data || w.ReadOperand(i.Dst, 0) != 0 {
		t.Fatal("writeback lost the original mask, offset or data")
	}
	if w.execution().OutstandingScalarMemAccess != 0 || w.execution().OutstandingVectorMemAccess != 0 ||
		w.execution().InFlightInsts != 0 || !w.cu.LDSUnit.IsIdle() {
		t.Fatal("LDS and wait counters did not drain")
	}
	assertLifecycleHolders(t, w.cu)
}
