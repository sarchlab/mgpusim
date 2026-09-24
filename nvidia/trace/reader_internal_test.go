package trace

import (
	"reflect"
	"testing"
)

type memFormatCase struct {
	name    string
	line    string
	addr    uint64
	suffix1 int
	suffix2 []int32
	imm     uint64
}

// memFormatCases covers the three address encodings of tracer versions 5
// and 6. Version 6 appends a second immediate and register values.
var memFormatCases = []memFormatCase{
	{
		name:    "v5 base and stride",
		line:    "00a0 ffffffff 1 R4 LDG.E 1 R4 4 1 0x7fb0fc430e00 4 0",
		addr:    0x7fb0fc430e00,
		suffix1: 4,
	},
	{
		name:    "v6 base and stride",
		line:    "0130 ffffffff 1 R11 LDG.E 1 R2 4 1 0x7f7b7dc40880 4 0 0 NoVal",
		addr:    0x7f7b7dc40880,
		suffix1: 4,
	},
	{
		name:    "v5 base and deltas",
		line:    "0100 00000007 1 R1 LDG.E 1 R2 4 2 0x1000 8 16 9",
		addr:    0x1000,
		suffix2: []int32{8, 16},
		imm:     9,
	},
	{
		name:    "v6 base and deltas with register values",
		line:    "0100 00000007 1 R1 LDG.E 1 R2 4 2 0x1000 8 16 5 0 Val 1 3 0x1 0x2 0x3",
		addr:    0x1000,
		suffix2: []int32{8, 16},
		imm:     5,
	},
	{
		name: "v6 address list",
		line: "0100 00000003 1 R1 LDG.E 1 R2 4 0 0x1000 0x2000 7 0 NoVal",
		addr: 0x1000,
		imm:  7,
	},
	{
		name: "no memory access",
		line: "0000 ffffffff 1 R1 LDC 0 0 3 0 NoVal",
		imm:  3,
	},
}

func TestExtractInstMemoryFormats(t *testing.T) {
	for _, tc := range memFormatCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := extractInst(tc.line, 0)

			if inst.MemAddress != tc.addr {
				t.Errorf("address: got %#x, want %#x", inst.MemAddress, tc.addr)
			}

			if inst.MemAddressSuffix1 != tc.suffix1 {
				t.Errorf("stride: got %d, want %d", inst.MemAddressSuffix1, tc.suffix1)
			}

			if !reflect.DeepEqual(inst.MemAddressSuffix2, tc.suffix2) {
				t.Errorf("deltas: got %v, want %v", inst.MemAddressSuffix2, tc.suffix2)
			}

			if inst.Immediate != tc.imm {
				t.Errorf("immediate: got %d, want %d", inst.Immediate, tc.imm)
			}
		})
	}
}
