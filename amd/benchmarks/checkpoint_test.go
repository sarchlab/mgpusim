package benchmarks

import (
	"bytes"
	"errors"
	"testing"
)

func TestCheckpointConfigAndTransactionalLoad(t *testing.T) {
	type state struct{ Requested, Effective int }
	value := state{Requested: 4}
	binds := 0
	saved := NewCheckpoint(&value, func() int { return value.Requested }, func() error {
		binds++
		if value.Effective < 0 {
			return errors.New("invalid binding")
		}
		return nil
	})
	var initial bytes.Buffer
	if err := saved.SaveCheckpoint(&initial); err != nil {
		t.Fatal(err)
	}
	value.Effective = 8
	var normalized bytes.Buffer
	if err := saved.SaveCheckpoint(&normalized); err != nil {
		t.Fatal(err)
	}
	mismatchState := state{Requested: 5}
	mismatch := NewCheckpoint(&mismatchState, func() int { return mismatchState.Requested }, nil)
	for _, data := range [][]byte{initial.Bytes(), normalized.Bytes()} {
		if err := mismatch.LoadCheckpoint(bytes.NewReader(data)); err == nil {
			t.Fatal("configuration mismatch accepted")
		}
		if mismatchState.Requested != 5 || mismatchState.Effective != 0 {
			t.Fatal("mismatch mutated state")
		}
	}
	value.Effective = 20
	if err := saved.LoadCheckpoint(bytes.NewReader(normalized.Bytes())); err != nil {
		t.Fatal(err)
	}
	if value.Effective != 8 || binds != 1 {
		t.Fatal("load did not replace live state and rebind")
	}
	if err := saved.LoadCheckpoint(bytes.NewBufferString("bad JSON")); err == nil {
		t.Fatal("bad JSON accepted")
	}
	if value.Effective != 8 || binds != 1 {
		t.Fatal("decode failure mutated live state")
	}
	value.Effective = -1
	var invalid bytes.Buffer
	if err := saved.SaveCheckpoint(&invalid); err != nil {
		t.Fatal(err)
	}
	value.Effective = 8
	if err := saved.LoadCheckpoint(bytes.NewReader(invalid.Bytes())); err == nil {
		t.Fatal("binding failure accepted")
	}
	if value.Effective != 8 || saved.Prepare() == nil {
		t.Fatal("failed binding left runnable object")
	}
	if err := saved.LoadCheckpoint(bytes.NewReader(initial.Bytes())); err != nil {
		t.Fatal(err)
	}
	if saved.Prepare() != nil || value.Effective != 0 {
		t.Fatal("valid reload did not recover object")
	}
}
