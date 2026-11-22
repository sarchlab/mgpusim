package insts

import (
	"encoding/binary"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestAMDGPUKernelRegisterNote(t *testing.T) {
	descriptor, err := msgpack.Marshal(map[string]any{
		"amdhsa.kernels": []amdGPUKernelMetadata{
			{Name: "first", SGPR: 22, VGPR: 27},
			{Name: "second", SGPR: 10, VGPR: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	const owner = "AMDGPU\x00"
	note := make([]byte, 12+8+((len(descriptor)+3)&^3))
	binary.LittleEndian.PutUint32(note, uint32(len(owner)))
	binary.LittleEndian.PutUint32(note[4:], uint32(len(descriptor)))
	binary.LittleEndian.PutUint32(note[8:], amdGPUMetadataNote)
	copy(note[12:], owner)
	copy(note[20:], descriptor)

	metadata := amdGPUCodeObjectMetadataInNotes(note, binary.LittleEndian)
	if metadata == nil {
		t.Fatal("valid AMDGPU metadata note was not decoded")
	}
	kernels := metadata.Kernels
	if len(kernels) != 2 || kernels[0].Name != "first" || kernels[1].Name != "second" {
		t.Fatalf("unexpected kernels: %+v", kernels)
	}

	meta := &KernelCodeObjectMeta{WFSgprCount: 8, WIVgprCount: 4}
	applyAMDGPUMetadataCounts(meta, kernels[0])
	if meta.WFSgprCount != 24 || meta.WIVgprCount != 28 {
		t.Fatalf("register allocation: SGPR=%d VGPR=%d", meta.WFSgprCount, meta.WIVgprCount)
	}
	if got := amdGPUCodeObjectMetadataInNotes(note[:len(note)-4], binary.LittleEndian); got != nil {
		t.Fatalf("truncated note decoded: %+v", got)
	}
}
