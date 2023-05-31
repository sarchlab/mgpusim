package org

import "github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"

// TimeTable is a table that records the minimum number of cycles between any
// two types of DRAM commands.
type TimeTable [][]TimeTableEntry

// TimeTableEntry is an entry in the TimeTable.
type TimeTableEntry struct {
	NextCmdKind       signal.CommandKind
	MinCycleInBetween int
}

func (t TimeTable) getTimeAfter(cmdKind signal.CommandKind) []TimeTableEntry {
	return t[cmdKind]
}

// MakeTimeTable creates a new TimeTable.
func MakeTimeTable() TimeTable {
	return make([][]TimeTableEntry, signal.NumCmdKind)
}

// Timing records all the timing-related parameters for a DRAM model.
type Timing struct {
	SameBank              TimeTable
	OtherBanksInBankGroup TimeTable
	SameRank              TimeTable
	OtherRanks            TimeTable
}
