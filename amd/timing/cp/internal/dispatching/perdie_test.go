package dispatching

import (
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Per-Die Algorithm", func() {
	var (
		ctrl         *gomock.Controller
		gridBuilder0 *MockGridBuilder
		gridBuilder1 *MockGridBuilder
		pool         *MockCUResourcePool
		cus          []*MockCUResource
		alg          *perDieAlgorithm
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		gridBuilder0 = NewMockGridBuilder(ctrl)
		gridBuilder1 = NewMockGridBuilder(ctrl)

		cus = make([]*MockCUResource, 4)
		for i := 0; i < 4; i++ {
			cus[i] = NewMockCUResource(ctrl)
			cus[i].EXPECT().DispatchingPort().
				Return(messaging.RemotePort("CUPort" + strconv.Itoa(i))).
				AnyTimes()
		}

		pool = NewMockCUResourcePool(ctrl)
		pool.EXPECT().NumCU().Return(len(cus)).AnyTimes()
		pool.EXPECT().GetCU(gomock.Any()).
			DoAndReturn(func(i int) resource.CUResource { return cus[i] }).
			AnyTimes()

		// 2 dies, 2 CUs each: die 0 -> CU{0,1}, die 1 -> CU{2,3}.
		alg = &perDieAlgorithm{
			cuPool:    pool,
			numDies:   2,
			numWG:     8,
			cusPerDie: 2,
			dies: []*dieState{
				{gridBuilder: gridBuilder0, numWGInDie: 4, firstCU: 0},
				{gridBuilder: gridBuilder1, numWGInDie: 4, firstCU: 2},
			},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("interleaves a die's work-groups round-robin over only that die's CUs", func() {
		wgA := kernels.NewWorkGroup()
		wgB := kernels.NewWorkGroup()

		gridBuilder0.EXPECT().NextWG().Return(wgA)
		cus[0].EXPECT().ReserveResourceForWG(wgA).
			Return([]resource.WfLocation{}, true)
		loc0 := alg.NextForDie(0)
		Expect(loc0.valid).To(BeTrue())
		Expect(loc0.cuID).To(Equal(0)) // die 0's first CU

		gridBuilder0.EXPECT().NextWG().Return(wgB)
		cus[1].EXPECT().ReserveResourceForWG(wgB).
			Return([]resource.WfLocation{}, true)
		loc1 := alg.NextForDie(0)
		Expect(loc1.valid).To(BeTrue())
		Expect(loc1.cuID).To(Equal(1)) // interleaved onto die 0's next CU

		Expect(alg.dies[0].dispatchedWG).To(Equal(2))
		Expect(alg.numDispatchedWG).To(Equal(2))
	})

	It("dispatches a die onto its own CU range only (die-level block)", func() {
		wg := kernels.NewWorkGroup()
		gridBuilder1.EXPECT().NextWG().Return(wg)
		cus[2].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, true)

		loc := alg.NextForDie(1)
		Expect(loc.valid).To(BeTrue())
		Expect(loc.cuID).To(Equal(2)) // die 1 starts at firstCU=2; never touches CU 0/1
	})

	It("returns invalid once a die's block is exhausted", func() {
		alg.dies[0].dispatchedWG = 4 // == numWGInDie

		loc := alg.NextForDie(0)

		Expect(loc.valid).To(BeFalse())
	})

	It("holds the pulled work-group and retries when no CU on the die is free", func() {
		wg := kernels.NewWorkGroup()
		gridBuilder0.EXPECT().NextWG().Return(wg) // pulled exactly once
		cus[0].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)
		cus[1].EXPECT().ReserveResourceForWG(wg).
			Return([]resource.WfLocation{}, false)

		loc := alg.NextForDie(0)

		Expect(loc.valid).To(BeFalse())
		Expect(alg.dies[0].currWG).To(Equal(wg)) // held, not lost
		Expect(alg.numDispatchedWG).To(Equal(0))
	})
})
