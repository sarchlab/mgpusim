package benchmarks

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/modeling"
)

// Checkpoint persists benchmark state and reconnects transient references after loading.
// The state address stays stable, including when loading into a used benchmark.
type Checkpoint[S any] struct {
	State     *S
	bind      func() error
	loadError error
}

func NewCheckpoint[S any](state *S, bind func() error) Checkpoint[S] {
	return Checkpoint[S]{State: state, bind: bind}
}

// Prepare reports a prior binding error before the next host step.
func (s *Checkpoint[S]) Prepare() error {
	return s.loadError
}
func (s *Checkpoint[S]) SaveCheckpoint(w io.Writer) error {
	if err := s.Prepare(); err != nil {
		return err
	}
	return modeling.WriteCheckpoint(w, s.State, nil)
}
func (s *Checkpoint[S]) LoadCheckpoint(r io.Reader) error {
	var next S
	if err := modeling.ReadCheckpoint(r, &next, nil); err != nil {
		return err
	}
	previous := *s.State
	*s.State = next
	if s.bind != nil {
		if err := s.bind(); err != nil {
			*s.State = previous
			s.loadError = fmt.Errorf("benchmark bindings: %w", err)
			return s.loadError
		}
	}
	s.loadError = nil
	return nil
}
