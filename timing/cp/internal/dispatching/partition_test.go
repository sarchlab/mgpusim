package dispatching

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/kernels"
	"github.com/sarchlab/mgpusim/v4/timing/cp/internal/resource"
)

var _ = Describe("Partition Algorithm", func() {
	var (
		ctrl         *gomock.Controller
		gridBuilder0 *MockGridBuilder
		gridBuilder1 *MockGridBuilder
		pool         *MockCUResourcePool
		cus          []*MockCUResource
		alg          *partitionAlgorithm
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		gridBuilder0 = NewMockGridBuilder(ctrl)
		gridBuilder1 = NewMockGridBuilder(ctrl)

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

		alg = &partitionAlgorithm{
			partitions: []*partition{
				{
					gridBuilder: gridBuilder0,
				},
				{
					gridBuilder: gridBuilder1,
				},
			},
			cuPool:            pool,
			numWG:             16,
			numWGPerPartition: 8,
			currWGs:           make([]*kernels.WorkGroup, 2),
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should check if there are more work-groups to generate", func() {
		alg.numDispatchedWG = 10
		alg.numWG = 10

		hasNext := alg.HasNext()

		Expect(hasNext).To(BeFalse())
	})

	It("should dispatch next wg", func() {
		wg := kernels.NewWorkGroup()

		gridBuilder0.EXPECT().NextWG().Return(wg)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		location := alg.Next()

		Expect(location.valid).To(BeTrue())
		Expect(alg.partitions[0].dispatchedWG).To(Equal(1))
		Expect(alg.currWGs[0]).To(BeNil())
		Expect(alg.numDispatchedWG).To(Equal(1))
		Expect(alg.nextPartition).To(Equal(1))
	})

	It("should return invalid location when dispatch is not possible", func() {
		wg := kernels.NewWorkGroup()

		alg.partitions[1].dispatchedWG = 8
		gridBuilder0.EXPECT().NextWG().Return(wg)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)
		cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)

		location := alg.Next()

		Expect(location.valid).To(BeFalse())
		Expect(alg.partitions[0].dispatchedWG).To(Equal(0))
		Expect(alg.numDispatchedWG).To(Equal(0))
	})

	It("should do work stealing", func() {
		wg := kernels.NewWorkGroup()

		alg.nextPartition = 1

		alg.partitions[1].dispatchedWG = 8
		alg.currWGs[0] = wg
		cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		location := alg.Next()

		Expect(location.valid).To(BeTrue())
		Expect(location.cuID).To(Equal(1))
		Expect(alg.partitions[0].dispatchedWG).To(Equal(1))
		Expect(alg.currWGs[0]).To(BeNil())
		Expect(alg.numDispatchedWG).To(Equal(1))
	})
})
