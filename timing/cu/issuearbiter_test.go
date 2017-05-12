package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		wfState := []WfState{
			WfRunning, WfFetched, WfFetched, WfReady, WfFetched,
			WfFetched, WfFetched, WfFetched, WfFetched, WfFetched,
		}
		IssueDirs := []IssueDirection{
			IssueDirVALU, IssueDirScalar, IssueDirVMem,
			IssueDirBranch, IssueDirLDS, IssueDirInternal,
			IssueDirVALU, IssueDirBranch, IssueDirVALU,
			IssueDirVMem,
		}
		wfs := make([]*Wavefront, 0)

		for i := 0; i < len(wfState); i++ {
			wf := new(Wavefront)
			wf.State = wfState[i]
			wf.IssueDir = IssueDirs[i]
			wfs = append(wfs, wf)
			wfPools[0].AddWf(wf)
		}

		issueCandidate := arbiter.Arbitrate(wfPools)

		Expect(len(issueCandidate)).To(Equal(6))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[0])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[1])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[2])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[3])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[4])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[5])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[6])))
		Expect(issueCandidate).To(ContainElement(BeIdenticalTo(wfs[7])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[8])))
		Expect(issueCandidate).NotTo(ContainElement(BeIdenticalTo(wfs[9])))
	})
})
