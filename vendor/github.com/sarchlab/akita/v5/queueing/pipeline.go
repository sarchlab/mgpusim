package queueing

// PipelineStage represents a single item occupying a lane and stage in a
// pipeline. It is JSON-serializable.
type PipelineStage[T any] struct {
	Lane      int `json:"lane"`
	Stage     int `json:"stage"`
	Item      T   `json:"item"`
	CycleLeft int `json:"cycle_left"`
}

// Pipeline is a generic multi-lane, multi-stage pipeline. It is a
// JSON-serializable value type.
type Pipeline[T any] struct {
	Width     int                `json:"width"`
	NumStages int                `json:"num_stages"`
	Stages    []PipelineStage[T] `json:"stages"`
}

// CanAccept returns true if there is at least one free lane at stage 0.
func (p *Pipeline[T]) CanAccept() bool {
	occupied := 0
	for i := range p.Stages {
		if p.Stages[i].Stage == 0 {
			occupied++
		}
	}

	return occupied < p.Width
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

	if p.Width <= len(usedSmall) {
		used = usedSmall[:p.Width]
	} else {
		used = make([]bool, p.Width)
	}

	for i := range p.Stages {
		if p.Stages[i].Stage == 0 {
			used[p.Stages[i].Lane] = true
		}
	}

	lane := 0
	for lane < p.Width {
		if !used[lane] {
			break
		}
		lane++
	}

	p.Stages = append(p.Stages, PipelineStage[T]{
		Lane:      lane,
		Stage:     0,
		Item:      item,
		CycleLeft: 0,
	})
}

// Tick advances the pipeline by one cycle. Items at the last stage with
// CycleLeft==0 are pushed into postBuf. Items at intermediate stages advance
// to the next stage if the next stage has a free lane. Stages are processed
// from last to first to prevent double-advancement.
//
// Returns true if any item moved.
func (p *Pipeline[T]) Tick(postBuf *Buffer[T]) bool {
	return p.TickFunc(func(item T) bool {
		if postBuf.CanPush() {
			postBuf.PushTyped(item)
			return true
		}
		return false
	})
}

// TickFunc advances the pipeline by one cycle, using the provided accept
// function for items that have completed all stages (CycleLeft==0 at last
// stage). The accept function should return true if it consumed the item.
//
// Items are processed from highest stage to lowest to prevent
// double-advancement within a single tick.
//
// Returns true if any item moved.
func (p *Pipeline[T]) TickFunc(accept func(T) bool) bool {
	n := len(p.Stages)
	if n == 0 {
		return false
	}

	moved := false
	lastStage := p.NumStages - 1

	// Phase 1: Handle items at the last stage.
	// Items with CycleLeft > 0 still need to wait (decrement their counter).
	// Items with CycleLeft == 0 are ready to be output.
	for i := n - 1; i >= 0; i-- {
		s := &p.Stages[i]
		if s.Stage == lastStage {
			if s.CycleLeft > 0 {
				s.CycleLeft--
				moved = true
			} else if accept(s.Item) {
				p.Stages[i] = p.Stages[n-1]
				n--
				moved = true
			}
		}
	}

	p.Stages = p.Stages[:n]

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
	n := len(p.Stages)
	moved := false

	minStage, maxStage := p.stageRange()

	// Cap maxStage: items at lastStage were already handled in Phase 1.
	lastStage := p.NumStages - 1
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
			s := &p.Stages[i]
			if s.Stage != stage {
				continue
			}

			if s.CycleLeft > 0 {
				s.CycleLeft--
				moved = true

				continue
			}

			nextStage := stage + 1
			nextIdx := (nextStage - occBase) * p.Width
			curIdx := (stage - occBase) * p.Width

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
	minStage := p.Stages[0].Stage
	maxStage := p.Stages[0].Stage

	for i := 1; i < len(p.Stages); i++ {
		st := p.Stages[i].Stage
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
	occSlots := occRange * p.Width

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

	for i := 0; i < len(p.Stages); i++ {
		s := &p.Stages[i]
		occ[(s.Stage-minStage)*p.Width+s.Lane] = true
	}

	return occ
}
