package cu

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"testing"
)

type failedDebugWriter struct {
	bytes.Buffer
	err error
}

func (w *failedDebugWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func TestISADebuggerCloseCompletesEmptyLog(t *testing.T) {
	var output bytes.Buffer
	h := NewISADebugger(log.New(&output, "", 0), nil)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	first := output.String()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 || output.String() != first {
		t.Fatal("invalid or duplicated empty output")
	}
}

func TestISADebuggerCloseReportsWriteError(t *testing.T) {
	want := errors.New("output failed")
	h := NewISADebugger(log.New(&failedDebugWriter{err: want}, "", 0), nil)
	for range 2 {
		if !errors.Is(h.Close(), want) {
			t.Fatal("lost output error")
		}
	}
}

func TestISADebuggerCloseReportsFooterError(t *testing.T) {
	var output failedDebugWriter
	h := NewISADebugger(log.New(&output, "", 0), nil)
	want := errors.New("footer failed")
	output.err = want
	for range 2 {
		if !errors.Is(h.Close(), want) {
			t.Fatal("lost footer error")
		}
	}
}
