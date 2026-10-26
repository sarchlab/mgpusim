package runtime

import (
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

type CommandKind uint8

const (
	CopyH2D CommandKind = iota
	CopyD2H
	LaunchKernel
	RecordEvent
	Noop
)

// Command contains an immutable submission descriptor. Kernel values are
// captured; copies refer to storage addresses. Ownership transfers from Runtime
// to component State when execution starts.
type Command struct {
	ID              CommandID
	Kind            CommandKind
	Src, Dst        Ptr
	Owned           []Ptr // Internal CPU staging allocations released at completion.
	ByteSize        int
	Code            insts.CodeObjectValue
	Packets         []kernels.PacketValue
	PacketAddresses []Ptr
	GPUs            []int
}
