package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

var _ = Describe("IssueArbiter", func() {
	var (
		arbiter *IssueArbiter
		wfPools []*WavefrontPool
	)

	BeforeEach(func() {
		arbiter = NewIssueArbiter(newTestComputeUnit("CU", nil))
		wfPools = make([]*WavefrontPool, 0, 4)
		for i := 0; i < 4; i++ {
			wfPools = append(wfPools, NewWavefrontPool(newTestComputeUnit("CU", nil), 0, 10))
		}
	})

	It("should decide which wf to issue", func() {
		wfState := []wavefront.WfState{
			wavefront.WfRunning, wavefront.WfReady, wavefront.WfReady, wavefront.WfReady, wavefront.WfReady,
			wavefront.WfReady, wavefront.WfReady, wavefront.WfReady, wavefront.WfReady, wavefront.WfReady,
		}
		exeUnits := []insts.ExeUnit{
			insts.ExeUnitVALU, insts.ExeUnitScalar, insts.ExeUnitVMem,
			insts.ExeUnitBranch, insts.ExeUnitLDS, insts.ExeUnitSpecial,
			insts.ExeUnitVALU, insts.ExeUnitBranch, insts.ExeUnitVALU,
			insts.ExeUnitVMem,
		}
		wfs := make([]*Wavefront, 0)

		for i := 0; i < len(wfState); i++ {
			wf := newTestWave()
			wf.execution().State = wfState[i]
			wfDescription := insts.NewInst()
			wfDescription.ExeUnit = exeUnits[i]
			wf.setTestNextInst(testInst(testIDs.NewID(), wfDescription))
			wfs = append(wfs, wf)
			testAddWave(wfPools[0], wf)

			if i == 3 || i == 6 {
				wf.setTestNextInst(nil)
			}
		}

		issueCandidate := arbiter.Arbitrate(wfPools)

		Expect(len(issueCandidate)).To(Equal(6))
		Expect(issueCandidate).NotTo(ContainElement(Equal(wfs[0])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[1])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[2])))
		Expect(issueCandidate).NotTo(ContainElement(Equal(wfs[3])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[4])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[5])))
		Expect(issueCandidate).NotTo(ContainElement(Equal(wfs[6])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[7])))
		Expect(issueCandidate).To(ContainElement(Equal(wfs[8])))
		Expect(issueCandidate).NotTo(ContainElement(Equal(wfs[9])))
	})

	It("should issue from all SIMDs with wavefronts", func() {
		// Put a VALU wavefront in SIMD 0
		wf0 := newTestWave()
		wf0.execution().State = wavefront.WfReady
		wf0Description := insts.NewInst()
		wf0Description.ExeUnit = insts.ExeUnitVALU
		wf0.setTestNextInst(testInst(testIDs.NewID(), wf0Description))
		testAddWave(wfPools[0], wf0)

		// Put a Scalar wavefront in SIMD 1
		wf1 := newTestWave()
		wf1.execution().State = wavefront.WfReady
		wf1Description := insts.NewInst()
		wf1Description.ExeUnit = insts.ExeUnitScalar
		wf1.setTestNextInst(testInst(testIDs.NewID(), wf1Description))
		testAddWave(wfPools[1], wf1)

		// Put a VMem wavefront in SIMD 2
		wf2 := newTestWave()
		wf2.execution().State = wavefront.WfReady
		wf2Description := insts.NewInst()
		wf2Description.ExeUnit = insts.ExeUnitVMem
		wf2.setTestNextInst(testInst(testIDs.NewID(), wf2Description))
		testAddWave(wfPools[2], wf2)

		// Put a Branch wavefront in SIMD 3
		wf3 := newTestWave()
		wf3.execution().State = wavefront.WfReady
		wf3Description := insts.NewInst()
		wf3Description.ExeUnit = insts.ExeUnitBranch
		wf3.setTestNextInst(testInst(testIDs.NewID(), wf3Description))
		testAddWave(wfPools[3], wf3)

		issueCandidate := arbiter.Arbitrate(wfPools)

		// All 4 wavefronts from all 4 SIMDs should be returned
		Expect(len(issueCandidate)).To(Equal(4))
		Expect(issueCandidate).To(ContainElement(Equal(wf0)))
		Expect(issueCandidate).To(ContainElement(Equal(wf1)))
		Expect(issueCandidate).To(ContainElement(Equal(wf2)))
		Expect(issueCandidate).To(ContainElement(Equal(wf3)))
	})
})
