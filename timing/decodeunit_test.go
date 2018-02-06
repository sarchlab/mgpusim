package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DecodeUnit", func() {
	var (
		du        *DecodeUnit
		execUnits []*mockCUComponent
	)

	BeforeEach(func() {
		du = NewDecodeUnit()
		execUnits = make([]*mockCUComponent, 4)
		for i := 0; i < 4; i++ {
			execUnits[i] = new(mockCUComponent)
			execUnits[i].canAccept = true
			du.AddExecutionUnit(execUnits[i])
		}
	})

	It("should tell if it cannot accept wave", func() {
		du.toDecode = new(Wavefront)
		Expect(du.CanAcceptWave()).To(BeFalse())
	})

	It("should tell if it can accept wave", func() {
		du.toDecode = nil
		Expect(du.CanAcceptWave()).To(BeTrue())
	})

	It("should accept wave", func() {
		wave := new(Wavefront)
		du.toDecode = nil
		du.AcceptWave(wave)
		Expect(du.toDecode).To(BeIdenticalTo(wave))
	})

	It("should return error if the decoder is busy", func() {
		wave := new(Wavefront)
		wave2 := new(Wavefront)
		du.toDecode = wave

		Expect(func() { du.AcceptWave(wave2) }).Should(Panic())
		Expect(du.toDecode).To(BeIdenticalTo(wave))
	})

	It("should deliver the wave to the execution unit", func() {
		wave := new(Wavefront)
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
		wave := new(Wavefront)
		wave.SIMDID = 1
		du.toDecode = wave
		execUnits[1].canAccept = false

		du.Run(10)

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
	})

})
