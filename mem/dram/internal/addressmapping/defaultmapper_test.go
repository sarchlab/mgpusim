package addressmapping

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Default Mapper", func() {
	var (
		mapper Mapper
		table  map[uint64]Location
	)

	BeforeEach(func() {
		mapper = MakeBuilder().Build()
		table = map[uint64]Location{
			0x0000_0000_0000_0000: {0, 0, 0, 0, 0, 0},
			0x0000_0000_0000_0040: {0, 0, 0, 0, 0, 1},
			0x0000_0000_0002_0040: {0, 0, 0, 0, 1, 1},
			0x0000_0000_0002_4040: {0, 0, 0, 1, 1, 1},
		}
	})

	It("should map", func() {
		for addr, location := range table {
			loc := mapper.Map(addr)
			Expect(loc).To(Equal(location))
		}
	})

})
