package bitops_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/sarchlab/mgpusim/v4/bitops"
)

var _ = Describe("Ops", func() {
	It("should extract bits from uint64", func() {
		table := []struct {
			input, output uint64
			lo, hi        int
		}{
			{0x0000_00FF_0000_0000, 0xFF, 32, 39},
		}

		for _, entry := range table {
			Expect(ExtractBitsFromU64(entry.input, entry.lo, entry.hi)).
				To(Equal(entry.output))
		}
	})

	It("should do sign extension", func() {
		table := []struct {
			input, output uint64
			signBit       int
		}{
			{0x0000_0000_0000_0001, 0x0000_0000_0000_0001, 2},
			{0x0000_0000_0000_0081, 0xffff_ffff_ffff_ff81, 7},
		}

		for _, entry := range table {
			Expect(SignExt(entry.input, entry.signBit)).
				To(Equal(entry.output))
		}
	})
})
