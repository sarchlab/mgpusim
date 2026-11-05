package insts_test

import (
	"encoding/binary"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func TestFlatAndGlobalPrinting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cdna3   bool
		segment uint32
		saddr   uint32
		load    string
		store   string
	}{
		{"flat_s0", true, 0, 0, "flat_load_dword v6, v[4:5]", "flat_store_dword v[4:5], v2"},
		{"flat_s6", true, 0, 6, "flat_load_dword v6, v[4:5]", "flat_store_dword v[4:5], v2"},
		{"flat_off", true, 0, 127, "flat_load_dword v6, v[4:5]", "flat_store_dword v[4:5], v2"},
		{"global_off", true, 2, 127, "global_load_dword v6, v[4:5], off", "global_store_dword v[4:5], v2, off"},
		{"global_s0", true, 2, 0, "global_load_dword v6, v4, s[0:1]", "global_store_dword v4, v2, s[0:1]"},
		{"global_s6", true, 2, 6, "global_load_dword v6, v4, s[6:7]", "global_store_dword v4, v2, s[6:7]"},
		{"gcn3_flat", false, 0, 0, "flat_load_dword v6, v[4:5]", "flat_store_dword v[4:5], v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, op := range []struct {
				name string
				bits uint32
				want string
			}{
				{"load", 0xdc500000, tc.load},
				{"store", 0xdc700000, tc.store},
			} {
				t.Run(op.name, func(t *testing.T) {
					buf := make([]byte, 8)
					binary.LittleEndian.PutUint32(buf, op.bits|tc.segment<<14)
					binary.LittleEndian.PutUint32(buf[4:], 0x06000204|tc.saddr<<16)
					d := insts.NewDisassembler()
					d.IsCDNA3 = tc.cdna3
					inst, err := d.Decode(buf)
					if err != nil {
						t.Fatal(err)
					}
					if got := insts.NewInstPrinter(nil).Print(inst); got != op.want {
						t.Fatalf("got %q, want %q", got, op.want)
					}
				})
			}
		})
	}
}
