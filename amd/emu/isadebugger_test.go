package emu

import (
	"bytes"
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
	h := &ISADebugger{Logger: log.New(&output, "", 0), isFirstEntry: true}
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

	for _, want := range []string{`"wg":[0,0,0],"wf":0,`, `"wg":[3,2,0],"wf":64,`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %s in debugger output: %q", want, output.String())
		}
	}
}
