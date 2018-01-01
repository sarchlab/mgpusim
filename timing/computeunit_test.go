package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type mockWGMapper struct {
	OK         bool
	UnmappedWg *WorkGroup
}

func (m *mockWGMapper) MapWG(req *gcn3.MapWGReq) bool {
	return m.OK
}

func (m *mockWGMapper) UnmapWG(wg *WorkGroup) {
	m.UnmappedWg = wg
}

var _ = Describe("ComputeUnit", func() {
	var (
		cu       *ComputeUnit
		wgMapper *mockWGMapper

		connection *core.MockConnection
	)

	BeforeEach(func() {
		wgMapper = new(mockWGMapper)
		cu = NewComputeUnit("cu")
		cu.wgMapper = wgMapper
		cu.Freq = 1

		connection = core.NewMockConnection()
		core.PlugIn(cu, "ToACE", connection)
	})

	Context("when processing MapWGReq", func() {
		It("should reply OK if mapping is successful", func() {
			wgMapper.OK = true

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg, nil)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg, nil)
			expectedResponse.Ok = true
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if mapping is failed", func() {
			wgMapper.OK = false

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg, nil)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg, nil)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})
	})
})
