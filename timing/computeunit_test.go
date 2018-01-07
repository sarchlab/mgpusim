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

type mockWfDispatcher struct {
	dispatchedWf *gcn3.DispatchWfReq
}

func (m *mockWfDispatcher) DispatchWf(req *gcn3.DispatchWfReq) {
	m.dispatchedWf = req
}

var _ = Describe("ComputeUnit", func() {
	var (
		cu           *ComputeUnit
		wgMapper     *mockWGMapper
		wfDispatcher *mockWfDispatcher

		connection *core.MockConnection
	)

	BeforeEach(func() {
		wgMapper = new(mockWGMapper)
		wfDispatcher = new(mockWfDispatcher)

		cu = NewComputeUnit("cu", nil)
		cu.WGMapper = wgMapper
		cu.WfDispatcher = wfDispatcher
		cu.Freq = 1

		connection = core.NewMockConnection()
		core.PlugIn(cu, "ToACE", connection)
	})

	Context("when processing MapWGReq", func() {
		It("should reply OK if mapping is successful", func() {
			wgMapper.OK = true

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = true
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if there are pending wavefronts", func() {
			cu.WfToDispatch = append(cu.WfToDispatch, new(WfDispatchInfo))

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if mapping is failed", func() {
			wgMapper.OK = false

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})
	})

	Context("when processing DispatchWfReq", func() {
		It("should dispatch wf", func() {
			req := gcn3.NewDispatchWfReq(nil, cu, 10, nil)
			cu.Handle(req)
			Expect(wfDispatcher.dispatchedWf).To(BeIdenticalTo(req))
		})

		It("should handle WfDispatchCompletionEvent", func() {
			evt := NewWfDispatchCompletionEvent(10, cu, nil)
			cu.Handle(evt)
		})
	})
})
