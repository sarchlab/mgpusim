package cu

// WavefrontPool is a view of one canonical pool membership list.
type WavefrontPool struct {
	Capacity int
	cu       *ComputeUnit
	index    int
}

func NewWavefrontPool(cu *ComputeUnit, index, capacity int) *WavefrontPool {
	return &WavefrontPool{Capacity: capacity, cu: cu, index: index}
}
func (p *WavefrontPool) waves() []*Wavefront {
	ids := p.cu.comp.State.Pools[p.index]
	out := make([]*Wavefront, len(ids))
	for i, id := range ids {
		out[i] = p.cu.wave(id)
	}
	return out
}
func (p *WavefrontPool) AddWf(w *Wavefront) {
	p.cu.comp.State.Pools[p.index] = append(p.cu.comp.State.Pools[p.index], p.cu.useWave(w))
}
func (p *WavefrontPool) Availability() int { return p.Capacity - len(p.cu.comp.State.Pools[p.index]) }
func (p *WavefrontPool) RemoveWf(w *Wavefront) {
	ids := p.cu.comp.State.Pools[p.index]
	for i, id := range ids {
		if id == w.handle {
			p.cu.comp.State.Pools[p.index] = append(ids[:i], ids[i+1:]...)
			return
		}
	}
}

// ids returns membership for an immediate operation, never for retention.
func (p *WavefrontPool) ids() []waveHandle { return p.cu.comp.State.Pools[p.index] }
