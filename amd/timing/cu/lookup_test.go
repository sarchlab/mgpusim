package cu

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

func TestWaveLookupRetirementAndSameIDRewind(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	w.SetPC(123)
	var checkpoint bytes.Buffer
	if err := c.comp.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	c.retireGroup(g.slot)
	if c.findWavefront(w.UID) != nil {
		t.Fatal("retired wave is still indexed")
	}
	next := checkpointGroup(c).waves()[0]
	if c.wavefrontByID(next.UID).handle != next.handle {
		t.Fatal("reused slot not indexed")
	}
	if err := c.comp.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err != nil {
		t.Fatal(err)
	}
	if c.wavefrontByID(w.UID).PC() != 123 || c.findWavefront(next.UID) != nil {
		t.Fatal("wave index survived canonical State rewind")
	}
	c.retireGroup(g.slot)
	if len(c.comp.State.WaveHandles) != 0 {
		t.Fatal("wave index grows after retirement")
	}
}

func TestVectorLookupMixedTypesRetirementAndRewind(t *testing.T) {
	c := checkpointCU(t)
	for i := uint64(1); i <= 16; i++ {
		entry := VectorAccessState{Inst: i}
		if i%2 == 0 {
			entry.Read = messaging.Msg{ID: i}
		} else {
			entry.Write = messaging.Msg{ID: i}
		}
		c.comp.State.Vector = append(c.comp.State.Vector, entry)
	}
	var checkpoint bytes.Buffer
	if err := c.comp.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	// A response of the wrong kind used to be ignored, and must remain so.
	c.handleVectorDataLoadReturn(messaging.Msg{RspTo: 3})
	c.handleVectorDataStoreRsp(messaging.Msg{RspTo: 4})
	if len(c.comp.State.Vector) != 16 {
		t.Fatal("wrong response kind removed a transaction")
	}
	// Stable out-of-order retirement preserves admission order for subsequent
	// lookup and checkpoint continuation (as in both response handlers).
	for _, id := range []uint64{14, 2, 9, 1} {
		i := c.vectorTransactionIndex(id)
		c.comp.State.Vector = append(c.comp.State.Vector[:i], c.comp.State.Vector[i+1:]...)
	}
	for _, id := range []uint64{3, 4, 8, 15, 16} {
		if c.vectorTransactionByID(id).Inst != id {
			t.Fatalf("lookup lost transaction %d", id)
		}
	}
	if err := c.comp.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err != nil {
		t.Fatal(err)
	}
	c.comp.State.Vector[13].Inst = 99
	if c.vectorTransactionByID(14).Inst != 99 {
		t.Fatal("lookup retained pre-rewind values")
	}
}

func BenchmarkVectorLookup(b *testing.B) {
	c := &ComputeUnit{comp: &Comp{}}
	for i := uint64(1); i <= 512; i++ {
		c.comp.State.Vector = append(c.comp.State.Vector, VectorAccessState{Read: messaging.Msg{ID: i}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = c.vectorTransactionByID(512)
	}
}

func TestVectorAdmissionRejectsUnorderedIDs(t *testing.T) {
	c := checkpointCU(t)
	c.admitVector(VectorAccessState{Read: messaging.Msg{ID: 7}})
	c.admitVector(VectorAccessState{Write: messaging.Msg{ID: 9}})
	for _, id := range []uint64{0, 7, 8, 9} {
		requireSlotPanic(t, func() { c.admitVector(VectorAccessState{Read: messaging.Msg{ID: id}}) })
	}
	if len(c.comp.State.Vector) != 2 || c.vectorTransactionByID(9).Write.ID != 9 {
		t.Fatal("failed admission changed live transactions")
	}
	c.admitVector(VectorAccessState{Read: messaging.Msg{ID: 10}})
	if c.vectorTransactionIndex(10) != 2 {
		t.Fatal("valid admission after rejection failed")
	}
}
