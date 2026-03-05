package insts

import (
	"encoding/binary"
	"testing"
)

func TestIsV2V3Header_ValidHeader(t *testing.T) {
	// Construct a minimal valid V2/V3 header (256 bytes)
	data := make([]byte, 512) // 256 header + some instructions

	// CodeVersionMajor = 1 (offset 0-3)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	// CodeVersionMinor = 2 (offset 4-7)
	binary.LittleEndian.PutUint32(data[4:8], 2)
	// MachineKind = 1 (offset 8-9)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	// MachineVersionMajor = 8 (GCN4, offset 10-11)
	binary.LittleEndian.PutUint16(data[10:12], 8)
	// KernelCodeEntryByteOffset = 256 (offset 16-23)
	binary.LittleEndian.PutUint64(data[16:24], 256)

	if !isV2V3Header(data) {
		t.Error("expected valid V2/V3 header to be detected")
	}
}

func TestIsV2V3Header_GCN3Header(t *testing.T) {
	// GCN3 (MachineVersionMajor = 8)
	data := make([]byte, 256)
	binary.LittleEndian.PutUint32(data[0:4], 1)   // major
	binary.LittleEndian.PutUint32(data[4:8], 0)    // minor
	binary.LittleEndian.PutUint16(data[8:10], 1)   // machineKind
	binary.LittleEndian.PutUint16(data[10:12], 8)  // machineVersionMajor
	binary.LittleEndian.PutUint64(data[16:24], 256) // entryOffset

	if !isV2V3Header(data) {
		t.Error("expected GCN3 header to be detected as V2/V3")
	}
}

func TestIsV2V3Header_TooShort(t *testing.T) {
	data := make([]byte, 100)
	if isV2V3Header(data) {
		t.Error("expected short data to NOT be detected as V2/V3")
	}
}

func TestIsV2V3Header_WrongMajor(t *testing.T) {
	data := make([]byte, 256)
	binary.LittleEndian.PutUint32(data[0:4], 2)   // wrong major
	binary.LittleEndian.PutUint32(data[4:8], 0)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	binary.LittleEndian.PutUint16(data[10:12], 8)
	binary.LittleEndian.PutUint64(data[16:24], 256)

	if isV2V3Header(data) {
		t.Error("expected wrong major version to NOT be V2/V3")
	}
}

func TestIsV2V3Header_WrongMachineVersion(t *testing.T) {
	// Machine version outside known range (e.g., 0 or 100)
	data := make([]byte, 256)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	binary.LittleEndian.PutUint32(data[4:8], 0)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	binary.LittleEndian.PutUint16(data[10:12], 0) // invalid machine version
	binary.LittleEndian.PutUint64(data[16:24], 256)

	if isV2V3Header(data) {
		t.Error("expected invalid machine version to NOT be V2/V3")
	}
}

func TestIsV2V3Header_WrongEntryOffset(t *testing.T) {
	// KernelCodeEntryByteOffset != 256
	data := make([]byte, 256)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	binary.LittleEndian.PutUint32(data[4:8], 0)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	binary.LittleEndian.PutUint16(data[10:12], 8)
	binary.LittleEndian.PutUint64(data[16:24], 128) // wrong offset

	if isV2V3Header(data) {
		t.Error("expected wrong entry offset to NOT be V2/V3")
	}
}

func TestIsV2V3Header_FalsePositivePrevention(t *testing.T) {
	// Simulate the stencil2d false positive scenario:
	// Raw V5 instruction bytes that happened to match old signature
	// 0x01000000 0x02000000 (codeVersionMajor=1, codeVersionMinor=2)
	// 0x0001 (machineKind=1)
	// But with machine version and entry offset that don't match V2/V3
	data := make([]byte, 256)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	binary.LittleEndian.PutUint32(data[4:8], 2)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	// These fields won't match valid V2/V3 for random instruction bytes
	binary.LittleEndian.PutUint16(data[10:12], 0) // not a valid GPU gen
	binary.LittleEndian.PutUint64(data[16:24], 0)  // not 256

	if isV2V3Header(data) {
		t.Error("expected V5 instruction bytes to NOT be falsely detected as V2/V3")
	}
}

func TestNewKernelCodeObjectFromEntireTextSection_V2V3(t *testing.T) {
	data := make([]byte, 512)
	binary.LittleEndian.PutUint32(data[0:4], 1)   // CodeVersionMajor
	binary.LittleEndian.PutUint32(data[4:8], 1)    // CodeVersionMinor
	binary.LittleEndian.PutUint16(data[8:10], 1)   // MachineKind
	binary.LittleEndian.PutUint16(data[10:12], 8)  // MachineVersionMajor
	binary.LittleEndian.PutUint64(data[16:24], 256) // EntryOffset

	// Put a pattern after the 256-byte header to verify we extract it
	data[256] = 0xAB
	data[257] = 0xCD

	co := newKernelCodeObjectFromEntireTextSection(data)

	if co.Version != CodeObjectV3 {
		t.Errorf("expected V3, got %d", co.Version)
	}
	if len(co.Data) != 256 {
		t.Errorf("expected 256 bytes of instruction data, got %d", len(co.Data))
	}
	if co.Data[0] != 0xAB || co.Data[1] != 0xCD {
		t.Error("instruction data not correctly extracted from after header")
	}
}

func TestNewKernelCodeObjectFromEntireTextSection_NonV2V3(t *testing.T) {
	// Data that doesn't match V2/V3 header
	data := make([]byte, 512)
	data[0] = 0xC0 // typical GPU instruction byte
	data[1] = 0x02

	co := newKernelCodeObjectFromEntireTextSection(data)

	if co.Version != CodeObjectV5 {
		t.Errorf("expected V5 fallback, got %d", co.Version)
	}
	if len(co.Data) != 512 {
		t.Errorf("expected full 512 bytes as instruction data, got %d", len(co.Data))
	}
}

func TestLoadKernelCodeObjectFromFS_V5(t *testing.T) {
	// Test loading a V5 (gfx942) kernel
	co := LoadKernelCodeObjectFromFS(
		"../../amd/benchmarks/shoc/stencil2d/kernels_gfx942.hsaco",
		"StencilKernel",
	)

	if co.Version != CodeObjectV5 {
		t.Errorf("expected V5, got %d", co.Version)
	}
	if co.Symbol == nil {
		t.Error("expected symbol to be set")
	}
	if co.Symbol.Name != "StencilKernel" {
		t.Errorf("expected symbol name 'StencilKernel', got '%s'", co.Symbol.Name)
	}
	// V5 kernel data should NOT have 256 bytes stripped
	if co.Symbol.Size != uint64(len(co.Data)) {
		t.Errorf("expected Data length (%d) to match symbol size (%d) for V5 kernel",
			len(co.Data), co.Symbol.Size)
	}
}

func TestLoadKernelCodeObjectFromFS_V2V3(t *testing.T) {
	// Test loading a V2/V3 (GCN3) kernel
	co := LoadKernelCodeObjectFromFS(
		"../../amd/benchmarks/shoc/stencil2d/kernels.hsaco",
		"StencilKernel",
	)

	if co.Version != CodeObjectV3 {
		t.Errorf("expected V3, got %d", co.Version)
	}
	if co.Symbol == nil {
		t.Error("expected symbol to be set")
	}
	// V2/V3 kernel data should have 256-byte header stripped
	expectedLen := int(co.Symbol.Size) - 256
	if len(co.Data) != expectedLen {
		t.Errorf("expected Data length %d (symbol size %d - 256), got %d",
			expectedLen, co.Symbol.Size, len(co.Data))
	}
}
