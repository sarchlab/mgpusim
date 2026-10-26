package runtime

import (
	"bytes"
	"testing"
)

func TestZeroByteStagingCompletesAfterRestore(t *testing.T) {
	f := newRuntimeFixture(t)
	f.r.stageUpload(f.q, 0, []byte{})
	marker := f.r.EnqueueRecordEvent(f.q)
	if len(f.r.queue(f.q).Pending[0].Owned) != 0 {
		t.Fatal("zero address was owned")
	}
	var saved bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	if err := f.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}
	cmd, ok := f.r.TakeCommand(f.q)
	if !ok {
		t.Fatal("zero-byte upload missing")
	}
	f.r.CompleteCommand(f.q, cmd, 0)
	f.r.CompleteEvents(f.q, 0)
	if f.r.Unfinished()[0] != 0 || len(f.r.queue(f.q).Pending) != 0 {
		t.Fatal("zero-byte upload did not complete")
	}
	if _, err := f.r.EventTime(marker); err != nil {
		t.Fatal("following marker did not complete", err)
	}
	if err := f.r.ReleaseEvent(marker); err != nil {
		t.Fatal(err)
	}
	if len(f.r.context(f.ctx).Buffers) != 0 {
		t.Fatal("zero-byte staging allocated memory")
	}
}
