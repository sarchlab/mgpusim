// Package pipelines defines a pipeline timing model.
package pipelines

import "gitlab.com/akita/akita"

// Pipeline is a conceptual model for pipelines. It is used when we do not
// really care about each stage in a pipeline, but just want to model the
// latency
type Pipeline interface {
	CanAccept(time akita.VTimeInSec, item interface{}) bool
	Accept(time akita.VTimeInSec, item interface{}) (cycleToExit int)

	SetNumStages(numStages int)
	SetStageLatency(latencyInCycles int)
	SetNumLines(numLines int)
	SetFrequency(freq akita.Freq)
}

func NewPipeline() Pipeline {
	p := new(impl)
	p.numLines = 1
	p.numStages = 5
	p.stageLatency = 1
	p.freq = 1 * akita.GHz
	return p
}

type impl struct {
	numStages    int
	stageLatency int
	numLines     int
	freq         akita.Freq

	injectTime []akita.VTimeInSec
}

func (p *impl) CanAccept(time akita.VTimeInSec, item interface{}) bool {
	cycle := p.freq.Cycle(time)
	busyCount := 0

	for _, iTime := range p.injectTime {
		iCycle := p.freq.Cycle(iTime)
		if int(cycle-iCycle) < p.stageLatency {
			busyCount++
		}
	}

	if busyCount >= p.numLines {
		return false
	}
	return true
}

func (p *impl) Accept(time akita.VTimeInSec, item interface{}) (cycleToExit int) {
	p.injectTime = append(p.injectTime, time)
	if len(p.injectTime) > 2 {
		p.injectTime = p.injectTime[1:]
	}

	cycles := p.numStages * p.stageLatency
	return cycles
}

func (p *impl) SetNumStages(numStages int) {
	p.numStages = numStages
}

func (p *impl) SetStageLatency(latencyInCycles int) {
	p.stageLatency = latencyInCycles
}

func (p *impl) SetNumLines(numLines int) {
	p.numLines = numLines
}

func (p *impl) SetFrequency(freq akita.Freq) {
	p.freq = freq
}
