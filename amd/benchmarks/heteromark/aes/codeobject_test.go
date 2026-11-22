package aes

import (
	"bytes"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func TestReadonlyCodeObjectPreservesTableDisplacement(t *testing.T) {
	const symbol = "aes256_encrypt_kernel"
	code := insts.LoadGFX942KernelWithReadOnlyData(cdna3HSACOBytes, symbol)
	plain := insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, symbol)
	snapshot := insts.CodeObjectSnapshot(code)
	code = snapshot.Restore()
	if entry := code.KernelCodeEntryByteOffset; entry == 0 || !bytes.Equal(code.Data[entry:], plain.Data) {
		t.Fatal("restored image lost the entry offset or instruction bytes")
	}
	table := bytes.Index(code.Data, s)
	if table < 0 || uint64(table) >= code.KernelCodeEntryByteOffset {
		t.Fatal("linked S-box was not preserved before the entry point")
	}
}
