package signal

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping"
)

// CommandKind represents the kind of the command.
type CommandKind int

// A list of supported DRAM command kinds.
const (
	CmdKindRead CommandKind = iota
	CmdKindReadPrecharge
	CmdKindWrite
	CmdKindWritePrecharge
	CmdKindActivate
	CmdKindPrecharge
	CmdKindRefreshBank
	CmdKindRefresh
	CmdKindSRefEnter
	CmdKindSRefExit
	NumCmdKind
)

var cmdKindString = map[CommandKind]string{
	CmdKindRead:           "Read",
	CmdKindReadPrecharge:  "ReadPrecharge",
	CmdKindWrite:          "Write",
	CmdKindWritePrecharge: "WritePrecharge",
	CmdKindActivate:       "Activate",
	CmdKindPrecharge:      "Precharge",
	CmdKindRefreshBank:    "RefreshBank",
	CmdKindRefresh:        "Refresh",
	CmdKindSRefEnter:      "SRefEnter",
	CmdKindSRefExit:       "SRefExit",
}

// String converts the command kind to the string representation.
func (k CommandKind) String() string {
	str, found := cmdKindString[k]

	if found {
		return str
	}

	return "Invalid"
}

// Command is a signal sent to the bank to let the bank perform a certain
// action.
type Command struct {
	addressmapping.Location
	ID        string
	Kind      CommandKind
	Address   uint64
	CycleLeft int
	SubTrans  *SubTransaction
}

// Clone will create another command with the same content, but different ID.
func (c *Command) Clone() *Command {
	newCmd := &Command{
		ID:        sim.GetIDGenerator().Generate(),
		Location:  c.Location,
		Kind:      c.Kind,
		Address:   c.Address,
		CycleLeft: c.CycleLeft,
		SubTrans:  c.SubTrans,
	}
	return newCmd
}

// IsRead returns true if the command is a read or read precharge command.
func (c *Command) IsRead() bool {
	return c.Kind == CmdKindRead || c.Kind == CmdKindReadPrecharge
}

// IsWrite returns true if the command is a write or write precharge command.
func (c *Command) IsWrite() bool {
	return c.Kind == CmdKindWrite || c.Kind == CmdKindWritePrecharge
}

// IsReadOrWrite returns true if the command is one of the following kind.
// * Read
// * Write
// * ReadPrecharge
// * WritePrecharge
func (c *Command) IsReadOrWrite() bool {
	return c.IsRead() || c.IsWrite()
}
