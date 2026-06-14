package queueing

// PipelineStage represents a single item occupying a lane and stage in a
// pipeline. It is a plain data record used to inspect pipeline occupancy.
type PipelineStage[T any] struct {
	Lane      int `json:"lane"`
	Stage     int `json:"stage"`
	Item      T   `json:"item"`
	CycleLeft int `json:"cycle_left"`
}

// Sink is the destination a Pipeline pushes completed items into. Both
// queueing.Buffer and any bounded collection that accepts items satisfy it.
type Sink[T any] interface {
	CanPush() bool
	PushTyped(T)
}

// Pipeline is a generic multi-lane, multi-stage pipeline. Its state is
// encapsulated; callers drive it through Accept/Tick and inspect it through
// the accessors.
type Pipeline[T any] struct {
	width     int
	numStages int
	stages    []PipelineStage[T]
}

// NewPipeline creates a pipeline with the given width (lanes) and number of
// stages. It returns a value so the pipeline can be embedded directly in a
// component's state.
func NewPipeline[T any](width, numStages int) Pipeline[T] {
	return Pipeline[T]{
		width:     width,
		numStages: numStages,
	}
}

// Stages returns a copy of the items currently occupying the pipeline. It is
// intended for inspection and testing; mutating the returned slice has no
// effect on the pipeline.
func (p *Pipeline[T]) Stages() []PipelineStage[T] {
	out := make([]PipelineStage[T], len(p.stages))
	copy(out, p.stages)

	return out
}

// Clear removes all items from the pipeline.
func (p *Pipeline[T]) Clear() {
	p.stages = nil
}

// CanAccept returns true if there is at least one free lane at stage 0.
func (p *Pipeline[T]) CanAccept() bool {
	occupied := 0
	for i := range p.stages {
		if p.stages[i].Stage == 0 {
			occupied++
		}
	}

	return occupied < p.width
}

// Accept inserts an item into the first stage of the pipeline. It occupies
// the next free lane. The item starts with CycleLeft=0, meaning it will
// attempt to advance to the next stage on the very next Tick. The total
// latency through the pipeline equals NumStages ticks.
func (p *Pipeline[T]) Accept(item T) {
	// Use a fixed-size bitset on the stack for small widths,
	// fall back to a slice for larger ones.
	var usedSmall [16]bool
	var used []bool

	if p.width <= len(usedSmall) {
		used = usedSmall[:p.width]
	} else {
		used = make([]bool, p.width)
	}

	for i := range p.stages {
		if p.stages[i].Stage == 0 {
			used[p.stages[i].Lane] = true
		}
	}

	lane := 0
	for lane < p.width {
		if !used[lane] {
			break
		}
		lane++
	}

	p.stages = append(p.stages, PipelineStage[T]{
		Lane:      lane,
		Stage:     0,
		Item:      item,
		CycleLeft: 0,
	})
}

// AcceptWithDelay inserts an item like Accept, but the item dwells delay extra
// cycles at stage 0 before it becomes eligible to advance. Accept(item) is
// equivalent to AcceptWithDelay(item, 0).
func (p *Pipeline[T]) AcceptWithDelay(item T, delay int) {
	p.Accept(item)
	p.stages[len(p.stages)-1].CycleLeft = delay
}

// Tick advances the pipeline by one cycle. Items at the last stage with
// CycleLeft==0 are pushed into sink. Items at intermediate stages advance
// to the next stage if the next stage has a free lane. Stages are processed
// from last to first to prevent double-advancement.
//
// Returns true if any item moved.
func (p *Pipeline[T]) Tick(sink Sink[T]) bool {
	n := len(p.stages)
	if n == 0 {
		return false
	}

	moved := false
	lastStage := p.numStages - 1

	// Phase 1: Try to output items at last stage with CycleLeft == 0.
	for i := n - 1; i >= 0; i-- {
		s := &p.stages[i]
		if s.Stage == lastStage && s.CycleLeft == 0 {
			if sink.CanPush() {
				sink.PushTyped(s.Item)
				p.stages[i] = p.stages[n-1]
				n--
				moved = true
			}
		}
	}

	p.stages = p.stages[:n]

	// Phase 2: Advance remaining items from high stage to low.
	if n > 0 {
		moved = p.advanceItems() || moved
	}

	return moved
}

// advanceItems processes all pipeline items, advancing them toward higher
// stages. Items are processed from highest stage to lowest to prevent
// double-advancement within a single tick.
func (p *Pipeline[T]) advanceItems() bool {
	n := len(p.stages)
	moved := false

	minStage, maxStage := p.stageRange()

	// Cap maxStage: items at lastStage were already handled in Phase 1.
	lastStage := p.numStages - 1
	if maxStage > lastStage-1 {
		maxStage = lastStage - 1
	}

	if maxStage < minStage {
		return moved
	}

	occ := p.buildOccupancy(minStage, maxStage+1)
	occBase := minStage

	for stage := maxStage; stage >= minStage; stage-- {
		for i := 0; i < n; i++ {
			s := &p.stages[i]
			if s.Stage != stage {
				continue
			}

			if s.CycleLeft > 0 {
				s.CycleLeft--
				moved = true

				continue
			}

			nextStage := stage + 1
			nextIdx := (nextStage - occBase) * p.width
			curIdx := (stage - occBase) * p.width

			if occ[nextIdx+s.Lane] {
				continue
			}

			occ[curIdx+s.Lane] = false
			s.Stage = nextStage
			occ[nextIdx+s.Lane] = true
			moved = true
		}
	}

	return moved
}

// stageRange returns the minimum and maximum stage numbers among items.
func (p *Pipeline[T]) stageRange() (int, int) {
	minStage := p.stages[0].Stage
	maxStage := p.stages[0].Stage

	for i := 1; i < len(p.stages); i++ {
		st := p.stages[i].Stage
		if st < minStage {
			minStage = st
		}

		if st > maxStage {
			maxStage = st
		}
	}

	return minStage, maxStage
}

// buildOccupancy builds a flat bool slice tracking which (stage, lane) slots
// are occupied, for stages in [minStage, maxStage+1].
func (p *Pipeline[T]) buildOccupancy(minStage, maxStage int) []bool {
	occRange := maxStage + 1 - minStage + 1 // +1 for nextStage lookups
	occSlots := occRange * p.width

	var occSmall [128]bool
	var occ []bool

	if occSlots <= len(occSmall) {
		occ = occSmall[:occSlots]
		for i := range occ {
			occ[i] = false
		}
	} else {
		occ = make([]bool, occSlots)
	}

	for i := 0; i < len(p.stages); i++ {
		s := &p.stages[i]
		occ[(s.Stage-minStage)*p.width+s.Lane] = true
	}

	return occ
}
