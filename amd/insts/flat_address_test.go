package insts_test

import (
	"encoding/binary"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func TestCDNA3FlatAndGlobalAddressing(t *testing.T) {
	// AMD MI300 ISA, section 10.1: FLAT ignores SADDR and uses a 12-bit
	// unsigned offset; GLOBAL uses SADDR and a 13-bit signed offset.
	for _, tc := range []struct {
		name     string
		segment  uint32
		saddr    uint32
		offset   uint32
		wantRegs int
		wantBase int64
		wantOff  uint32
	}{
		{"flat_zero_saddr", 0, 0, 0, 2, 127, 0},
		{"flat_nonzero_saddr", 0, 6, 0, 2, 127, 0},
		{"flat_off", 0, 127, 0, 2, 127, 0},
		{"flat_unsigned_offset", 0, 0, 0x1ffc, 2, 127, 0xffc},
		{"global_s0", 2, 0, 4, 1, 0, 4},
		{"global_s6", 2, 6, 0, 1, 6, 0},
		{"global_off_negative_offset", 2, 127, 0x1ffc, 2, 127, 0xfffffffc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint32(buf, 0xdc500000|tc.segment<<14|tc.offset)
			binary.LittleEndian.PutUint32(buf[4:], 0x02000002|tc.saddr<<16)
			d := insts.NewDisassembler()
			d.IsCDNA3 = true
			inst, err := d.Decode(buf)
			if err != nil {
				t.Fatal(err)
			}
			if inst.Addr.RegCount != tc.wantRegs || inst.SAddr.IntValue != tc.wantBase || inst.Offset0 != tc.wantOff {
				t.Fatalf("address regs=%d, scalar base=%d, offset=%#x; want %d, %d, %#x",
					inst.Addr.RegCount, inst.SAddr.IntValue, inst.Offset0, tc.wantRegs, tc.wantBase, tc.wantOff)
			}
		})
	}
}
