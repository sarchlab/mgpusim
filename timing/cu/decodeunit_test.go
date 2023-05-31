package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
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

	It("should tell if it cannot accept wave", func() {
		du.toDecode = new(wavefront.Wavefront)
		Expect(du.CanAcceptWave()).To(BeFalse())
	})

	It("should tell if it can accept wave", func() {
		du.toDecode = nil
		Expect(du.CanAcceptWave()).To(BeTrue())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		du.toDecode = nil
		du.AcceptWave(wave, 10)
		Expect(du.toDecode).To(BeIdenticalTo(wave))
	})

	It("should return error if the decoder is busy", func() {
		wave := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		du.toDecode = wave

		Expect(func() { du.AcceptWave(wave2, 10) }).Should(Panic())
		Expect(du.toDecode).To(BeIdenticalTo(wave))
	})

	It("should deliver the wave to the execution unit", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.toDecode = wave

		du.Run(10)

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.toDecode).To(BeNil())
	})

	It("should not deliver to the execution unit, if busy", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.toDecode = wave
		execUnits[1].canAccept = false

		du.Run(10)

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
	})
	It("should flush the decode unit", func() {
		wave := new(wavefront.Wavefront)
		wave.SIMDID = 1
		du.toDecode = wave

		du.Flush()

		Expect(du.toDecode).To(BeNil())
	})

})
