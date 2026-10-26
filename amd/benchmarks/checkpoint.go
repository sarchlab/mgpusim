package benchmarks

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/modeling"
)

// Checkpoint persists the exact state used by a benchmark. Configure reads requested
// settings before defaults are normalized; bind reconnects only transient code
// and interface references after loading. Neither callback is serialized.
// The state address stays stable, including when loading into a used benchmark.
type Checkpoint[C, S any] struct {
	State     *S
	Config    C
	configure func() C
	bind      func() error
	captured  bool
	loadError error
}

func NewCheckpoint[C, S any](state *S, configure func() C, bind func() error) Checkpoint[C, S] {
	return Checkpoint[C, S]{State: state, configure: configure, bind: bind}
}

// Prepare captures configuration before the first host step changes defaults.
func (s *Checkpoint[C, S]) Prepare() error {
	if s.loadError != nil {
		return s.loadError
	}
	if !s.captured {
		s.Config = s.configure()
		s.captured = true
	}
	return nil
}
func (s *Checkpoint[C, S]) SaveCheckpoint(w io.Writer) error {
	if err := s.Prepare(); err != nil {
		return err
	}
	return modeling.WriteCheckpoint(w, s.Config, s.State, nil)
}
func (s *Checkpoint[C, S]) LoadCheckpoint(r io.Reader) error {
	if !s.captured {
		s.Config = s.configure()
		s.captured = true
	}
	var next S
	if err := modeling.ReadCheckpoint(r, s.Config, &next, nil); err != nil {
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
