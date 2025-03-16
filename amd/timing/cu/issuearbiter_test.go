package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
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
			wfPools = append(wfPools, NewWavefrontPool(10))
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
			wf := new(wavefront.Wavefront)
			wf.State = wfState[i]
			wf.InstToIssue = wavefront.NewInst(insts.NewInst())
			wf.InstToIssue.ExeUnit = exeUnits[i]
			wfs = append(wfs, wf)
			wfPools[0].AddWf(wf)

			if i == 3 || i == 6 {
				wf.InstToIssue = nil
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
})
