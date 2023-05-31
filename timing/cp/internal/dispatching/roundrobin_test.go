package dispatching

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/kernels"
	"github.com/sarchlab/mgpusim/v3/timing/cp/internal/resource"
)

var _ = Describe("Round Robin Algorithm", func() {
	var (
		ctrl        *gomock.Controller
		gridBuilder *MockGridBuilder
		pool        *MockCUResourcePool
		cus         []*MockCUResource
		alg         *roundRobinAlgorithm
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		gridBuilder = NewMockGridBuilder(ctrl)

		cus = make([]*MockCUResource, 2)
		for i := 0; i < 2; i++ {
			cus[i] = NewMockCUResource(ctrl)
			cus[i].EXPECT().DispatchingPort().Return(nil).AnyTimes()
		}

		pool = NewMockCUResourcePool(ctrl)
		pool.EXPECT().NumCU().Return(len(cus)).AnyTimes()
		pool.EXPECT().
			GetCU(gomock.Any()).
			DoAndReturn(func(i int) resource.CUResource {
				return cus[i]
			}).
			AnyTimes()

		alg = &roundRobinAlgorithm{
			gridBuilder: gridBuilder,
			cuPool:      pool,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should start a new kernel", func() {
		var info kernels.KernelLaunchInfo

		gridBuilder.EXPECT().SetKernel(gomock.Any()).
			Do(func(i kernels.KernelLaunchInfo) {
				Expect(i).To(Equal(info))
			})

		alg.StartNewKernel(info)
	})

	It("should check if there are more work-groups to generate", func() {
		alg.numDispatchedWGs = 10
		gridBuilder.EXPECT().NumWG().Return(10)

		hasNext := alg.HasNext()

		Expect(hasNext).To(BeFalse())
	})

	It("should dispatch next wg", func() {
		wg := kernels.NewWorkGroup()

		alg.nextCU = 0

		gridBuilder.EXPECT().NextWG().Return(wg)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		location := alg.Next()

		Expect(location.valid).To(BeTrue())
		Expect(alg.numDispatchedWGs).To(Equal(1))
	})

	It("should return invalid location is dispatch is not possible", func() {
		wg := kernels.NewWorkGroup()

		alg.nextCU = 1

		gridBuilder.EXPECT().NextWG().Return(wg)
		call1 := cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false).
			After(call1)

		location := alg.Next()

		Expect(location.valid).To(BeFalse())
		Expect(alg.numDispatchedWGs).To(Equal(0))
	})
})
