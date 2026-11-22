package spmv

import "testing"

func TestCDNA3LargeCSRBufferSizes(t *testing.T) {
	const rows = 8_388_608
	items := cdna3ItemCount(rows, 64)
	values, vector, offsets := csrBufferBytes(rows, items)
	if items != 536_870_912 || values != 1<<31 || vector != 1<<25 || offsets != (1<<25)+4 {
		t.Fatalf("collected case produced %d items with %d, %d and %d byte buffers", items, values, vector, offsets)
	}
	t.Run("rejects signed ABI overflow", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("signed CSR index overflow accepted")
			}
		}()
		cdna3ItemCount(rows, 256)
	})
}
