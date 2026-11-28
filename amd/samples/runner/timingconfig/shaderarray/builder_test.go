package shaderarray

import (
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// buildPanic builds a shader array with the configuration and returns what
// the build panicked with, or nil.
func buildPanic(t *testing.T, configure func(Builder) Builder) (cause any) {
	t.Helper()
	s := simulation.MakeBuilder().Build()
	t.Cleanup(func() { _ = s.Terminate() })
	mapper := &mem.AddressMapper{Destinations: []messaging.RemotePort{"L2.Top"}}
	b := MakeBuilder().WithSimulation(s).WithNumCUs(1).
		WithL1AddressMapper(mapper).WithL1TLBAddressMapper(mapper)

	defer func() { cause = recover() }()
	configure(b).Build("SA")

	return nil
}

// The shader array hands every fetch setting to its compute units, which
// reject the combinations that do not work instead of falling back to
// fetching whole lines.
func TestShaderArrayRejectsInvalidInstructionFetch(t *testing.T) {
	cases := []struct {
		name      string
		configure func(Builder) Builder
		want      string
	}{
		{"prefetch without block fetch", func(b Builder) Builder {
			return b.WithInstPrefetchLines(8)
		}, "require InstBlockByteSize"},
		{"negative block size", func(b Builder) Builder {
			return b.WithInstBlockFetch(-32, 2, 16)
		}, "must be non-negative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cause := buildPanic(t, c.configure)
			if msg, _ := cause.(string); !strings.Contains(msg, c.want) {
				t.Fatalf("the build panicked with %v, want a panic about %q", cause, c.want)
			}
		})
	}

	if cause := buildPanic(t, func(b Builder) Builder {
		return b.WithInstBlockFetch(32, 2, 16).WithInstPrefetchLines(8)
	}); cause != nil {
		t.Fatalf("a valid block-fetch configuration panicked: %v", cause)
	}
}
