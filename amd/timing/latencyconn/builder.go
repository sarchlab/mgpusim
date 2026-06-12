package latencyconn

import "github.com/sarchlab/akita/v5/sim"

// Builder can help building a LatencyConnection.
type Builder struct {
	engine            sim.Engine
	freq              sim.Freq
	latency           int
	crossGroupPenalty int
}

// MakeBuilder creates a new Builder with default values.
func MakeBuilder() Builder {
	return Builder{
		latency: 1,
	}
}

// WithEngine sets the engine to use.
func (b Builder) WithEngine(e sim.Engine) Builder {
	b.engine = e
	return b
}

// WithFreq sets the frequency of the connection.
func (b Builder) WithFreq(f sim.Freq) Builder {
	b.freq = f
	return b
}

// WithLatency sets the latency in cycles for message delivery.
func (b Builder) WithLatency(n int) Builder {
	b.latency = n
	return b
}

// WithCrossGroupPenalty sets the extra latency (in cycles) added when the
// source and destination ports belong to different XCD groups.
func (b Builder) WithCrossGroupPenalty(n int) Builder {
	b.crossGroupPenalty = n
	return b
}

// Build creates a new LatencyConnection Comp.
func (b Builder) Build(name string) *Comp {
	c := &Comp{
		ports: ports{
			ports:   make([]sim.Port, 0),
			portMap: make(map[sim.RemotePort]int),
		},
		latency:           b.latency,
		crossGroupPenalty: b.crossGroupPenalty,
		portXCD:           make(map[sim.RemotePort]int),
		buffer:            make([]bufferedMsg, 0),
	}
	c.TickingComponent = sim.NewSecondaryTickingComponent(
		name, b.engine, b.freq, c)

	mw := &middleware{
		Comp: c,
	}
	c.AddMiddleware(mw)

	return c
}
