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
		arbiter = NewIssueArbiter()
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
		wfs := make([]*wavefront.Wavefront, 0)

		for i := 0; i < len(wfState); i++ {
			wf := newTestWave()
			wf.State = wfState[i]
			wf.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.NextInst().ExeUnit = exeUnits[i]
			wfs = append(wfs, wf)
			testAddWave(wfPools[0], wf)

			if i == 3 || i == 6 {
				wf.SetNextInst(nil)
			}
		}

		issueCandidate := arbiter.Arbitrate(wfPools)

		Expect(len(issueCandidate)).To(Equal(6))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[0])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[1])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[2])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[3])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[4])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[5])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[6])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[7])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[8])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[9])))
	})

	It("should issue from all SIMDs with wavefronts", func() {
		// Put a VALU wavefront in SIMD 0
		wf0 := newTestWave()
		wf0.State = wavefront.WfReady
		wf0.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf0.NextInst().ExeUnit = insts.ExeUnitVALU
		testAddWave(wfPools[0], wf0)

		// Put a Scalar wavefront in SIMD 1
		wf1 := newTestWave()
		wf1.State = wavefront.WfReady
		wf1.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf1.NextInst().ExeUnit = insts.ExeUnitScalar
		testAddWave(wfPools[1], wf1)

		// Put a VMem wavefront in SIMD 2
		wf2 := newTestWave()
		wf2.State = wavefront.WfReady
		wf2.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf2.NextInst().ExeUnit = insts.ExeUnitVMem
		testAddWave(wfPools[2], wf2)

		// Put a Branch wavefront in SIMD 3
		wf3 := newTestWave()
		wf3.State = wavefront.WfReady
		wf3.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf3.NextInst().ExeUnit = insts.ExeUnitBranch
		testAddWave(wfPools[3], wf3)

		issueCandidate := arbiter.Arbitrate(wfPools)

		// All 4 wavefronts from all 4 SIMDs should be returned
		Expect(len(issueCandidate)).To(Equal(4))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wf0)))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wf1)))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wf2)))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wf3)))
	})
})
