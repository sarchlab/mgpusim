package cu

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

func TestCodeObjectBorrowResolvesCurrentStateWithoutAllocation(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	if w.codeObject() != &g.raw().CodeObject {
		t.Fatal("code object was reconstructed")
	}
	if n := testing.AllocsPerRun(1000, func() { _ = w.codeObject() }); n != 0 {
		t.Fatalf("metadata borrow allocated %g times", n)
	}
	g.raw().CodeObject.Meta.KernelCodeEntryByteOffset = 256
	old := replaceCUState(t, c)
	g.raw().CodeObject.Meta.KernelCodeEntryByteOffset = 512
	g.raw().CodeObject.Version = insts.CodeObjectV5
	g.raw().CodeObject.Data = []byte{1, 2, 3}
	current := w.codeObject()
	if current != &g.raw().CodeObject || current.Meta.KernelCodeEntryByteOffset != 512 ||
		current.Version != insts.CodeObjectV5 || !bytes.Equal(current.Data, []byte{1, 2, 3}) {
		t.Fatal("existing wave view did not borrow replacement code object")
	}
	if old.Groups[g.slot].Raw.CodeObject.Meta.KernelCodeEntryByteOffset != 256 ||
		len(old.Groups[g.slot].Raw.CodeObject.Data) != 0 {
		t.Fatal("metadata replacement mutated old State")
	}
}

func TestCodeObjectBorrowRequiresPresentMetadata(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	g.raw().CodeObject.Present = false
	requireSlotPanic(t, func() { w.codeObject() })
	g.raw().CodeObject.Present = true
	g.raw().CodeObject.HasMeta = false
	requireSlotPanic(t, func() { w.codeObject() })
	g.raw().CodeObject.HasMeta = true
	if w.codeObject() != &g.raw().CodeObject {
		t.Fatal("present zero metadata was rejected")
	}
}

func TestDispatcherReadsReplacementMetadataAndVersion(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	g.raw().SizeX, g.raw().SizeY, g.raw().SizeZ = 8, 8, 1
	code := &g.raw().CodeObject
	code.Version = insts.CodeObjectV3
	code.Meta.KernelCodeEntryByteOffset = 256
	code.Meta.ComputePgmRsrc2 = 1 << 11 // enable separate work-item Y register
	code.Meta.MachineVersionMajor = 9
	w := g.waves()[0]
	d := NewWfDispatcher(c)
	d.DispatchWf(w, protocol.WfDispatchLocation{})
	if w.ReadOperand(insts.NewVRegOperand(0, 0, 1), 9) != 1 ||
		w.ReadOperand(insts.NewVRegOperand(0, 1, 1), 9) != 1 || w.PC() != w.packet().KernelObject+256 {
		t.Fatal("V3 metadata did not initialize separate X/Y registers or entry PC")
	}
	old := replaceCUState(t, c)
	g.raw().CodeObject.Version = insts.CodeObjectV5
	g.raw().CodeObject.Meta.KernelCodeEntryByteOffset = 512
	oldRegisters := bytes.Clone(old.VRegisters[0])
	d.DispatchWf(w, protocol.WfDispatchLocation{})
	if w.ReadOperand(insts.NewVRegOperand(0, 0, 1), 9) != 1025 || w.PC() != w.packet().KernelObject+512 {
		t.Fatal("replacement V5 metadata did not initialize packed IDs or entry PC")
	}
	if !bytes.Equal(old.VRegisters[0], oldRegisters) ||
		old.Waves[slotIndex(w.handle)].Execution.PC != w.packet().KernelObject+256 {
		t.Fatal("dispatch wrote through detached State")
	}
}
