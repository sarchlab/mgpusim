package pipelines_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/timing/pipelines"
)

var _ = Describe("Pipeline", func() {
	var (
		pipeline pipelines.Pipeline
	)

	BeforeEach(func() {
		pipeline = pipelines.NewPipeline()
		pipeline.SetNumLines(2)
		pipeline.SetNumStages(5)
		pipeline.SetStageLatency(2)
	})

	It("should accept", func() {
		cycles := pipeline.Accept(0, 1)
		Expect(cycles).To(Equal(10))
	})

	It("should not accept if the first stage is busy", func() {
		pipeline.Accept(0, 1)
		pipeline.Accept(0, 1)

		Expect(pipeline.CanAccept(0, 1)).To(BeFalse())
	})

})
