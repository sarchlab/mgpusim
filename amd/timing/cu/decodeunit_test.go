package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DecodeUnit", func() {
	var (
		cu        *ComputeUnit
		du        *DecodeUnit
		execUnits []*mockCUComponent
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
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
			du.state().ToDecode = append(du.state().ToDecode, testWaveID(cu, newTestWave()))
		}
		Expect(du.CanAcceptWave()).To(BeFalse())
	})

	It("should tell if it can accept wave when queue has room", func() {
		Expect(du.CanAcceptWave()).To(BeTrue())
	})

	It("should accept wave", func() {
		wave := newTestWave()
		registerTestWave(cu, wave)
		du.AcceptWave(wave)
		Expect(du.state().ToDecode).To(HaveLen(1))
		Expect(cu.wave(du.state().ToDecode[0])).To(Equal(wave))
	})

	It("should accept multiple waves", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		registerTestWave(cu, wave1)
		du.AcceptWave(wave1)
		registerTestWave(cu, wave2)
		du.AcceptWave(wave2)
		Expect(du.state().ToDecode).To(HaveLen(2))
	})

	It("should panic if the queue is full", func() {
		for i := 0; i < 4; i++ {
			wave := newTestWave()
			registerTestWave(cu, wave)
			du.AcceptWave(wave)
		}
		Expect(func() { wave := newTestWave(); registerTestWave(cu, wave); du.AcceptWave(wave) }).Should(Panic())
	})

	It("should deliver the wave to the execution unit", func() {
		wave := newTestWave()
		wave.execution().SIMDID = 1
		registerTestWave(cu, wave)
		du.AcceptWave(wave)

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.state().ToDecode).To(BeEmpty())
	})

	It("should deliver multiple waves to different execution units", func() {
		wave1 := newTestWave()
		wave1.execution().SIMDID = 0
		wave2 := newTestWave()
		wave2.execution().SIMDID = 2
		registerTestWave(cu, wave1)
		du.AcceptWave(wave1)
		registerTestWave(cu, wave2)
		du.AcceptWave(wave2)

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(1))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.state().ToDecode).To(BeEmpty())
	})

	It("should not deliver to the execution unit, if busy", func() {
		wave := newTestWave()
		wave.execution().SIMDID = 1
		registerTestWave(cu, wave)
		du.AcceptWave(wave)
		execUnits[1].canAccept = false

		du.Run()

		Expect(len(execUnits[0].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[1].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[2].acceptedWave)).To(Equal(0))
		Expect(len(execUnits[3].acceptedWave)).To(Equal(0))
		Expect(du.state().ToDecode).To(HaveLen(1))
	})

	It("should flush the decode unit", func() {
		wave := newTestWave()
		wave.execution().SIMDID = 1
		registerTestWave(cu, wave)
		du.AcceptWave(wave)

		du.Flush()

		Expect(du.state().ToDecode).To(BeEmpty())
	})

})
