package emugpu

import (
	"errors"
	"os"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// isaDebugOutput owns the file opened by the GPU builder. Its embedded hook
// logs instructions; Close completes the log before releasing the file.
type isaDebugOutput struct {
	*emu.ISADebugger
	file     *os.File
	closed   bool
	closeErr error
}

func (o *isaDebugOutput) Close() error {
	if !o.closed {
		o.closed = true
		o.closeErr = errors.Join(o.ISADebugger.Close(), o.file.Close())
	}
	return o.closeErr
}

// CloseISADebuggers completes and closes the ISA logs opened by GPU builders.
// Call after execution has stopped. Repeated calls return the original errors.
func CloseISADebuggers(s *sim.Simulation) error {
	var err error
	for _, component := range s.Components() {
		cu, ok := component.(*emu.Comp)
		if !ok {
			continue
		}
		for _, hook := range cu.Hooks() {
			if output, ok := hook.(*isaDebugOutput); ok {
				err = errors.Join(err, output.Close())
			}
		}
	}
	return err
}
