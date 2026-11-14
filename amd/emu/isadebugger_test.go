package emu

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

func TestISADebuggerLogsEveryWavefront(t *testing.T) {
	inst, err := insts.NewDisassembler().Decode([]byte{0, 0, 0x80, 0xbf}) // s_nop 0
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	h := NewISADebugger(log.New(&output, "", 0))
	codeObject := &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}

	for _, id := range [][3]int{{0, 0, 0}, {3, 2, 64}} {
		wg := kernels.NewWorkGroup(1)
		wg.IDX, wg.IDY = id[0], id[1]
		raw := kernels.NewWavefront(2)
		raw.WG = wg
		raw.CodeObject = codeObject
		raw.FirstWiFlatID = id[2]
		wf := NewWavefront(raw)
		wf.inst = inst

		h.Func(hooking.HookCtx{Item: wf, Detail: inst})
	}

	closeDebuggerAndCheckJSON(t, h, &output, 2)

	for _, want := range []string{`"wg":[0,0,0],"wf":0,`, `"wg":[3,2,0],"wf":64,`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %s in debugger output: %q", want, output.String())
		}
	}
}

func TestISADebuggerCloseCompletesEmptyLog(t *testing.T) {
	var output bytes.Buffer
	h := NewISADebugger(log.New(&output, "", 0))
	closeDebuggerAndCheckJSON(t, h, &output, 0)
}

func closeDebuggerAndCheckJSON(t *testing.T, h *ISADebugger, output *bytes.Buffer, want int) {
	t.Helper()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	first := output.String()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if output.String() != first {
		t.Fatal("closing twice changed the log")
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != want {
		t.Fatalf("got %d entries, want %d", len(rows), want)
	}
}

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

func TestISADebuggerCloseReportsWriteError(t *testing.T) {
	want := errors.New("output failed")
	h := NewISADebugger(log.New(&failedDebugWriter{err: want}, "", 0))
	for range 2 {
		if !errors.Is(h.Close(), want) {
			t.Fatal("lost output error")
		}
	}
}

func TestISADebuggerCloseReportsFooterError(t *testing.T) {
	var output failedDebugWriter
	h := NewISADebugger(log.New(&output, "", 0))
	want := errors.New("footer failed")
	output.err = want
	for range 2 {
		if !errors.Is(h.Close(), want) {
			t.Fatal("lost footer error")
		}
	}
}
