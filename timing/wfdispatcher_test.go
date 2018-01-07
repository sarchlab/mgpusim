package timing

import (
	. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"
)

//
var _ = Describe("WfDispatcher", func() {
	var (
		wfDispatcher *WfDispatcherImpl
	)

	BeforeEach(func() {
		wfDispatcher = new(WfDispatcherImpl)
	})

	It("should dispatch wavefront", func() {

	})
})
