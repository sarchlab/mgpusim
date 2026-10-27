package dispatching

import (
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
	"go.uber.org/mock/gomock"
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
			cus[i].EXPECT().DispatchingPort().
				Return(messaging.RemotePort("CUPort" + strconv.Itoa(i))).
				AnyTimes()
		}

		pool = NewMockCUResourcePool(ctrl)
		pool.EXPECT().NumCU().Return(len(cus)).AnyTimes()
		pool.EXPECT().
			GetCU(gomock.Any()).
			DoAndReturn(func(i int) resource.CUResource {
				return cus[i]
			}).
			AnyTimes()

		state := &AlgorithmState{Partitions: make([]PartitionState, 2), NumWG: 16, WGPerPartition: 8}
		alg = &partitionAlgorithm{cuPool: pool,
			state: func() *AlgorithmState { return state },
			gridForPartition: func(i int) kernels.GridBuilder {
				if i == 0 {
					return gridBuilder0
				}
				return gridBuilder1
			}}

	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should check if there are more work-groups to generate", func() {
		alg.state().Dispatched = 10
		alg.state().NumWG = 10

		hasNext := alg.HasNext()

		Expect(hasNext).To(BeFalse())
	})

	It("should dispatch next wg", func() {
		wg := kernels.NewWorkGroup(testNewID())
		wg.Wavefronts = nil
		wg.WorkItems = nil

		gridBuilder0.EXPECT().NextWG().Return(wg)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		location := alg.Next()

		Expect(location.valid).To(BeTrue())
		Expect(alg.state().Partitions[0].Dispatched).To(Equal(1))
		Expect(currentWG(alg.state().Partitions[0].Current)).To(BeNil())
		Expect(alg.state().Dispatched).To(Equal(1))
		Expect(alg.state().NextPartition).To(Equal(1))
	})

	It("should return invalid location when dispatch is not possible", func() {
		wg := kernels.NewWorkGroup(testNewID())
		wg.Wavefronts = nil
		wg.WorkItems = nil

		alg.state().Partitions[1].Dispatched = 8
		gridBuilder0.EXPECT().NextWG().Return(wg)
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)
		cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)

		location := alg.Next()

		Expect(location.valid).To(BeFalse())
		Expect(alg.state().Partitions[0].Dispatched).To(Equal(0))
		Expect(alg.state().Dispatched).To(Equal(0))
	})

	It("should do work stealing", func() {
		wg := kernels.NewWorkGroup(testNewID())
		wg.Wavefronts = nil
		wg.WorkItems = nil

		alg.state().NextPartition = 1

		alg.state().Partitions[1].Dispatched = 8
		alg.state().Partitions[0].Current = retainWG(wg)
		cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		location := alg.Next()

		Expect(location.valid).To(BeTrue())
		Expect(location.cuID).To(Equal(1))
		Expect(alg.state().Partitions[0].Dispatched).To(Equal(1))
		Expect(currentWG(alg.state().Partitions[0].Current)).To(BeNil())
		Expect(alg.state().Dispatched).To(Equal(1))
	})
})
