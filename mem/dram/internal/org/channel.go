package org

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// Banks is indexed by rank, bank-group, bank.
type Banks [][][]Bank

// GetSize returns the number of ranks, bank-groups, and banks.
func (b Banks) GetSize() (rank, bankGroup, bank uint64) {
	return uint64(len(b)), uint64(len(b[0])), uint64(len(b[0][0]))
}

// GetBank returns a specific bank identified by the rank index, bank-group
// index, and the bank index.
func (b Banks) GetBank(rank, bankGroup, bank uint64) Bank {
	return b[rank][bankGroup][bank]
}

// MakeBanks create all the banks.
func MakeBanks(numRank, numBankGroup, numBank uint64) Banks {
	b := make(Banks, numRank)

	for i := uint64(0); i < numRank; i++ {
		b[i] = make([][]Bank, numBankGroup)

		for j := uint64(0); j < numBankGroup; j++ {
			b[i][j] = make([]Bank, numBank)

			for k := uint64(0); k < numBank; k++ {
				b[i][j][k] = NewBankImpl("")
			}
		}
	}

	return b
}

// A Channel is a group of ranks.
type Channel interface {
	GetReadyCommand(
		now sim.VTimeInSec,
		cmd *signal.Command,
	) *signal.Command

	StartCommand(
		now sim.VTimeInSec,
		cmd *signal.Command,
	)

	UpdateTiming(
		now sim.VTimeInSec,
		cmd *signal.Command,
	)

	Tick(now sim.VTimeInSec) (madeProgress bool)
}

// ChannelImpl implements a Channel.
type ChannelImpl struct {
	Banks  Banks
	Timing Timing
}

// Tick updates the internal states of the channel.
func (cs *ChannelImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	for i := 0; i < len(cs.Banks); i++ {
		for j := 0; j < len(cs.Banks[0]); j++ {
			for k := 0; k < len(cs.Banks[0][0]); k++ {
				madeProgress = cs.Banks[i][j][k].Tick(now) || madeProgress
			}
		}
	}

	return madeProgress
}

// GetReadyCommand returns the command that is ready to start in the channel.
func (cs *ChannelImpl) GetReadyCommand(
	now sim.VTimeInSec,
	cmd *signal.Command,
) *signal.Command {
	readyCmd := cs.Banks.
		GetBank(cmd.Rank, cmd.BankGroup, cmd.Bank).
		GetReadyCommand(now, cmd)

	return readyCmd
}

// StartCommand starts a command in a bank.
func (cs *ChannelImpl) StartCommand(now sim.VTimeInSec, cmd *signal.Command) {
	cs.Banks.
		GetBank(cmd.Rank, cmd.BankGroup, cmd.Bank).
		StartCommand(now, cmd)
}

// UpdateTiming updates the timing-related states of the banks.
func (cs *ChannelImpl) UpdateTiming(now sim.VTimeInSec, cmd *signal.Command) {
	switch cmd.Kind {
	case signal.CmdKindActivate:
		fallthrough
	case signal.CmdKindRead, signal.CmdKindReadPrecharge,
		signal.CmdKindWrite, signal.CmdKindWritePrecharge,
		signal.CmdKindPrecharge, signal.CmdKindRefreshBank:
		cs.updateAllBankTiming(now, cmd)
	}
}

func (cs *ChannelImpl) updateAllBankTiming(
	now sim.VTimeInSec,
	cmd *signal.Command,
) {
	rank, bankGroup, bank := cs.Banks.GetSize()
	for i := uint64(0); i < rank; i++ {
		for j := uint64(0); j < bankGroup; j++ {
			for k := uint64(0); k < bank; k++ {
				cs.updateBankTiming(now, cmd, i, j, k)
			}
		}
	}
}

func (cs *ChannelImpl) updateBankTiming(
	now sim.VTimeInSec,
	cmd *signal.Command,
	rank, bankGroup, bank uint64,
) {
	timingTable := cs.Timing.OtherRanks
	if cmd.Rank == rank {
		timingTable = cs.Timing.SameRank

		if cmd.BankGroup == bankGroup {
			timingTable = cs.Timing.OtherBanksInBankGroup

			if cmd.Bank == bank {
				timingTable = cs.Timing.SameBank
			}
		}
	}

	for _, entry := range timingTable[cmd.Kind] {
		cs.Banks.GetBank(rank, bankGroup, bank).
			UpdateTiming(entry.NextCmdKind, entry.MinCycleInBetween)
	}
}
