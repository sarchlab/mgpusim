package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

var _ = Describe("DecodeUnit", func() {
	var (
		cu        *ComputeUnit
		du        *DecodeUnit
		execUnits []*mockCUComponent
	)

	BeforeEach(func() {
		cu = NewComputeUnit("CU", nil)
		du = NewDecodeUnit(cu)
		execUnits = make([]*mockCUComponent, 4)
		for i := 0; i < 4; i++ {
			execUnits[i] = new(mockCUComponent)
			execUnits[i].canAccept = true
			du.AddExecutionUnit(execUnits[i])
		}
	})

	It("should tell if it cannot accept wave when queue is full", func() {
		for i := 0; i < 4; i++ {
			du.toDecode = append(du.toDecode, new(wavefront.Wavefront))
		}
		Expect(du.CanAcceptWave()).To(BeFalse())
	})

	It("should tell if it can accept wave when queue has room", func() {
		Expect(du.CanAcceptWave()).To(BeTrue())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		du.AcceptWave(wave)
		Expect(du.toDecode).To(HaveLen(1))
		Expect(du.toDecode[0]).To(BeIdenticalTo(wave))
	})

	It("should accept multiple waves", func() {
		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		du.AcceptWave(wave1)
		du.AcceptWave(wave2)
		Expect(du.toDecode).To(HaveLen(2))
	})

	It("should panic if the queue is full", func() {
		for i := 0; i < 4; i++ {
			du.AcceptWave(new(wavefront.Wavefront))
		}
		Expect(func() { du.AcceptWave(new(wavefront.Wavefront)) }).Should(Panic())
	})

	It("should deliver the wave to the execution unit", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.AcceptWave(wave)

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.toDecode).To(BeEmpty())
	})

	It("should deliver multiple waves to different execution units", func() {
		wave1 := new(wavefront.Wavefront)
		wave1.SIMDID = 0
		wave2 := new(wavefront.Wavefront)
		wave2.SIMDID = 2
		du.AcceptWave(wave1)
		du.AcceptWave(wave2)

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.toDecode).To(BeEmpty())
	})

	It("should not deliver to the execution unit, if busy", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.AcceptWave(wave)
		execUnits[1].canAccept = false

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.toDecode).To(HaveLen(1))
	})

	It("should flush the decode unit", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.AcceptWave(wave)

		du.Flush()

		Expect(du.toDecode).To(BeEmpty())
	})

})
