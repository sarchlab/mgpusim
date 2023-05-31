package org

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// BankState represents the current state of a bank.
type BankState int

// A list of possible bank states.
const (
	BankStateOpen BankState = iota
	BankStateClosed
	BankStateSRef
	BankStatePD
	BankStateInvalid
)

// BankImpl provides a basic implementation of a bank.
type BankImpl struct {
	sim.HookableBase
	BankName             string
	state                BankState
	currentCmd           *signal.Command
	openRow              uint64
	CmdCycles            map[signal.CommandKind]int
	cyclesToCmdAvailable map[signal.CommandKind]int
}

// NewBankImpl creates a new BankImpl.
func NewBankImpl(name string) *BankImpl {
	b := &BankImpl{
		BankName:             name,
		state:                BankStateClosed,
		cyclesToCmdAvailable: make(map[signal.CommandKind]int),
		CmdCycles:            make(map[signal.CommandKind]int),
	}

	return b
}

// Name returns the name of the Bank.
func (b *BankImpl) Name() string {
	return b.BankName
}

// Tick updates the internal states of the bank.
func (b *BankImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = b.countDownCurrentCmd(now) || madeProgress
	madeProgress = b.countDownTiming() || madeProgress

	return madeProgress
}

func (b *BankImpl) countDownTiming() (madeProgress bool) {
	for i := range b.cyclesToCmdAvailable {
		if b.cyclesToCmdAvailable[i] > 0 {
			b.cyclesToCmdAvailable[i]--
			madeProgress = true
		}
	}
	return madeProgress
}

func (b *BankImpl) countDownCurrentCmd(now sim.VTimeInSec) (madeProgress bool) {
	if b.currentCmd != nil {
		b.currentCmd.CycleLeft--
		if b.currentCmd.CycleLeft <= 0 {
			b.completeCurrentCmd(now)
		}

		madeProgress = true
	}

	return madeProgress
}

func (b *BankImpl) completeCurrentCmd(now sim.VTimeInSec) {
	b.currentCmd.CycleLeft = 0

	tracing.EndTask(b.currentCmd.ID, b)

	if b.currentCmd.IsReadOrWrite() {
		b.currentCmd.SubTrans.Completed = true

		tracing.EndTask(b.currentCmd.SubTrans.ID, b)
	}

	// fmt.Printf("%.10f, %s, cmd completed, %s\n",
	// 	now, b.Name(), b.currentCmd.Kind.String())

	b.currentCmd = nil
}

// GetReadyCommand returns the next command is ready to be issued.
func (b *BankImpl) GetReadyCommand(
	now sim.VTimeInSec,
	cmd *signal.Command,
) *signal.Command {
	requiredKind := b.getRequiredCommandKind(cmd)
	if requiredKind == signal.NumCmdKind {
		panic("never")
	}

	if b.cyclesToCmdAvailable[requiredKind] == 0 {
		readyCmd := cmd.Clone()
		readyCmd.Kind = requiredKind
		return readyCmd
	}

	return nil
}

func (b *BankImpl) getRequiredCommandKind(cmd *signal.Command) signal.CommandKind {
	key := cmdKindTableKey{b.state, cmd.Kind}

	kindFunc, found := requiredCmdKindTable[key]
	if !found {
		return signal.NumCmdKind
	}

	return kindFunc(b, cmd)
}

// StartCommand starts a new command in the Bank.
func (b *BankImpl) StartCommand(now sim.VTimeInSec, cmd *signal.Command) {
	if b.currentCmd != nil {
		panic("previous cmd is not completed")
	}
	b.currentCmd = cmd
	b.currentCmd.CycleLeft = b.CmdCycles[cmd.Kind]

	key := cmdKindTableKey{b.state, cmd.Kind}

	updateFunc, found := stateUpdateTable[key]
	if !found {
		panic("never")
	}

	updateFunc(b, cmd)

	tracing.StartTask(
		cmd.ID,
		cmd.SubTrans.ID,
		b,
		"cmd",
		cmd.Kind.String(),
		nil,
	)

	// fmt.Printf("%.10f, %s, cmd started, %s\n",
	// 	now, b.Name(), b.currentCmd.Kind.String())
}

// UpdateTiming updates timing related states of the bank.
func (b *BankImpl) UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int) {
	t := b.cyclesToCmdAvailable[cmdKind]

	if t < cycleNeeded {
		b.cyclesToCmdAvailable[cmdKind] = cycleNeeded
	}

	//fmt.Printf("%s, cmd timing updated, %s, %d, %d\n",
	//	b.Name(), cmdKind.String(),
	//	cycleNeeded, b.cyclesToCmdAvailable[cmdKind])
}

type cmdKindTableKey struct {
	bankState BankState
	cmdKind   signal.CommandKind
}

type requiredCmdKindFunc func(b *BankImpl, cmd *signal.Command) signal.CommandKind
type updateStateFunc func(b *BankImpl, cmd *signal.Command)

var requiredCmdKindTable map[cmdKindTableKey]requiredCmdKindFunc
var stateUpdateTable map[cmdKindTableKey]updateStateFunc

func returnCmdKindActive(b *BankImpl, cmd *signal.Command) signal.CommandKind {
	return signal.CmdKindActivate
}

func actionOnOpenRowOrPrecharge(
	b *BankImpl,
	cmd *signal.Command,
) signal.CommandKind {
	if b.openRow == cmd.Row {
		return cmd.Kind
	}
	return signal.CmdKindPrecharge
}

func openRow(b *BankImpl, cmd *signal.Command) {
	b.openRow = cmd.Row
	b.state = BankStateOpen
}

func closeRow(b *BankImpl, cmd *signal.Command) {
	b.state = BankStateClosed
}

func doNothing(b *BankImpl, cmd *signal.Command) {
	// Do nothing
}

func init() {
	requiredCmdKindTable = map[cmdKindTableKey]requiredCmdKindFunc{
		{BankStateClosed, signal.CmdKindRead}:           returnCmdKindActive,
		{BankStateClosed, signal.CmdKindReadPrecharge}:  returnCmdKindActive,
		{BankStateClosed, signal.CmdKindWrite}:          returnCmdKindActive,
		{BankStateClosed, signal.CmdKindWritePrecharge}: returnCmdKindActive,
		{BankStateOpen, signal.CmdKindRead}:             actionOnOpenRowOrPrecharge,
		{BankStateOpen, signal.CmdKindReadPrecharge}:    actionOnOpenRowOrPrecharge,
		{BankStateOpen, signal.CmdKindWrite}:            actionOnOpenRowOrPrecharge,
		{BankStateOpen, signal.CmdKindWritePrecharge}:   actionOnOpenRowOrPrecharge,
	}

	stateUpdateTable = map[cmdKindTableKey]updateStateFunc{
		{BankStateClosed, signal.CmdKindActivate}:     openRow,
		{BankStateOpen, signal.CmdKindPrecharge}:      closeRow,
		{BankStateOpen, signal.CmdKindReadPrecharge}:  closeRow,
		{BankStateOpen, signal.CmdKindWritePrecharge}: closeRow,
		{BankStateOpen, signal.CmdKindRead}:           doNothing,
		{BankStateOpen, signal.CmdKindWrite}:          doNothing,
	}
}
