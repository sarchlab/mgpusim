package dram

// tickBanks updates all bank states: counts down timers and completes commands.
// Returns true if any progress was made.
func tickBanks(spec *Spec, cmdCycles map[commandKind]int, state *State) bool {
	madeProgress := false

	for i := range state.BankStates.Entries {
		bs := &state.BankStates.Entries[i].Data
		madeProgress = tickBank(state, bs) || madeProgress
	}

	return madeProgress
}

// tickBank updates a single bank's state.
func tickBank(state *State, bs *bankState) bool {
	madeProgress := false

	// Count down current command
	if bs.HasCurrentCmd {
		bs.CurrentCmd.CycleLeft--
		if bs.CurrentCmd.CycleLeft <= 0 {
			completeCommand(state, bs)
		}
		madeProgress = true
	}

	// Count down timing constraints
	for k, v := range bs.CyclesToCmdAvailable {
		if v > 0 {
			bs.CyclesToCmdAvailable[k] = v - 1
			madeProgress = true
		}
	}

	return madeProgress
}

// completeCommand finishes the current command on a bank.
func completeCommand(state *State, bs *bankState) {
	cmd := &bs.CurrentCmd
	cmd.CycleLeft = 0

	kind := commandKind(cmd.Kind)
	if isReadOrWrite(kind) {
		// Mark the sub-transaction as completed.
		// The ref may have been invalidated (TransIndex == -1) if the
		// parent transaction was already removed by respondMW. In that
		// case there is nothing to mark.
		ref := cmd.SubTransRef
		if ref.TransIndex >= 0 && ref.TransIndex < len(state.Transactions) &&
			ref.SubIndex >= 0 && ref.SubIndex < len(state.Transactions[ref.TransIndex].SubTransactions) {
			state.Transactions[ref.TransIndex].
				SubTransactions[ref.SubIndex].Completed = true
		}
	}

	bs.HasCurrentCmd = false
	bs.CurrentCmd = commandState{}
}

// isReadOrWrite returns true if the command kind is a read/write variant.
func isReadOrWrite(kind commandKind) bool {
	return kind == cmdKindRead || kind == cmdKindReadPrecharge ||
		kind == cmdKindWrite || kind == cmdKindWritePrecharge
}

// getReadyCommand checks if a command can be issued to the bank.
// It returns a copy of the command with the required kind, or nil.
func getReadyCommand(spec *Spec, state *State, bs *bankState, cmd *commandState) *commandState {
	requiredKind := getRequiredCommandKind(bs, cmd)
	if requiredKind == numCmdKind {
		return nil
	}

	key := cmdKindToString(requiredKind)
	if bs.CyclesToCmdAvailable[key] == 0 {
		// Check tFAW for activate commands
		if requiredKind == cmdKindActivate && spec.TFAW > 0 {
			if !canActivateUnderTFAW(spec, state, int(cmd.Location.Rank)) {
				return nil
			}
		}
		readyCmd := cloneCommand(cmd)
		readyCmd.Kind = int(requiredKind)
		return readyCmd
	}

	return nil
}

// canActivateUnderTFAW checks whether issuing an activate on the given rank
// would violate the tFAW constraint.
func canActivateUnderTFAW(spec *Spec, state *State, rank int) bool {
	history := findActivateHistory(&state.BankStates, rank)
	if history == nil {
		return true
	}
	stamps := history.Timestamps
	if len(stamps) < 4 {
		return true
	}
	// Check if the oldest of the last 4 activates is more than tFAW ago
	oldest := stamps[len(stamps)-4]
	return state.TickCount-oldest >= uint64(spec.TFAW)
}

// findActivateHistory returns a pointer to the rankActivateHistory for the
// given rank, or nil if not found.
func findActivateHistory(flat *bankStatesFlat, rank int) *rankActivateHistory {
	for i := range flat.ActivateHistories {
		if flat.ActivateHistories[i].Rank == rank {
			return &flat.ActivateHistories[i]
		}
	}
	return nil
}

// getRequiredCommandKind determines what command kind is actually needed
// given the bank state and the requested command.
func getRequiredCommandKind(bs *bankState, cmd *commandState) commandKind {
	bankSt := bankStateKind(bs.State)
	cmdKind := commandKind(cmd.Kind)

	type key struct {
		state bankStateKind
		kind  commandKind
	}

	switch (key{bankSt, cmdKind}) {
	case key{bankStateClosed, cmdKindRead},
		key{bankStateClosed, cmdKindReadPrecharge},
		key{bankStateClosed, cmdKindWrite},
		key{bankStateClosed, cmdKindWritePrecharge}:
		return cmdKindActivate

	case key{bankStateOpen, cmdKindRead},
		key{bankStateOpen, cmdKindReadPrecharge},
		key{bankStateOpen, cmdKindWrite},
		key{bankStateOpen, cmdKindWritePrecharge}:
		if bs.OpenRow == cmd.Location.Row {
			return cmdKind
		}
		return cmdKindPrecharge

	default:
		return numCmdKind
	}
}

// startCommand starts a command on a bank.
func startCommand(cmdCycles map[commandKind]int, state *State, bs *bankState, cmd *commandState) {
	if bs.HasCurrentCmd {
		panic("previous cmd is not completed")
	}

	bs.HasCurrentCmd = true
	bs.CurrentCmd = *cmd

	kind := commandKind(cmd.Kind)
	cycles, ok := cmdCycles[kind]
	if ok {
		bs.CurrentCmd.CycleLeft = cycles
	}

	// Track statistics
	switch kind {
	case cmdKindRead, cmdKindReadPrecharge:
		state.TotalReadCommands++
	case cmdKindWrite, cmdKindWritePrecharge:
		state.TotalWriteCommands++
	case cmdKindActivate:
		state.TotalActivates++
	case cmdKindPrecharge:
		state.TotalPrecharges++
	}

	// Update bank state based on the command
	bankSt := bankStateKind(bs.State)

	type key struct {
		state bankStateKind
		kind  commandKind
	}

	switch (key{bankSt, kind}) {
	case key{bankStateClosed, cmdKindActivate}:
		bs.OpenRow = cmd.Location.Row
		bs.State = int(bankStateOpen)
		recordActivateTimestamp(state, int(cmd.Location.Rank))
	case key{bankStateOpen, cmdKindPrecharge},
		key{bankStateOpen, cmdKindReadPrecharge},
		key{bankStateOpen, cmdKindWritePrecharge}:
		bs.State = int(bankStateClosed)
	case key{bankStateOpen, cmdKindRead},
		key{bankStateOpen, cmdKindWrite}:
		// Do nothing
	}
}

// updateTiming updates timing constraints across all banks after a command
// is issued.
func updateTiming(timing dramTiming, state *State, cmd *commandState) {
	kind := commandKind(cmd.Kind)

	switch kind {
	case cmdKindActivate,
		cmdKindRead, cmdKindReadPrecharge,
		cmdKindWrite, cmdKindWritePrecharge,
		cmdKindPrecharge, cmdKindRefreshBank:
		updateAllBankTiming(timing, state, cmd)
	}
}

// updateAllBankTiming iterates over all banks and applies timing constraints.
func updateAllBankTiming(timing dramTiming, state *State, cmd *commandState) {
	kind := commandKind(cmd.Kind)
	flat := &state.BankStates

	for i := range flat.Entries {
		entry := &flat.Entries[i]
		rank := uint64(entry.Rank)
		bankGroup := uint64(entry.BankGroup)
		bank := uint64(entry.BankIndex)

		var timingTable timeTable
		if cmd.Location.Rank == rank {
			if cmd.Location.BankGroup == bankGroup {
				if cmd.Location.Bank == bank {
					timingTable = timing.SameBank
				} else {
					timingTable = timing.OtherBanksInBankGroup
				}
			} else {
				timingTable = timing.SameRank
			}
		} else {
			timingTable = timing.OtherRanks
		}

		if int(kind) < len(timingTable) {
			for _, te := range timingTable[kind] {
				key := cmdKindToString(te.NextCmdKind)
				if entry.Data.CyclesToCmdAvailable == nil {
					entry.Data.CyclesToCmdAvailable = make(map[string]int)
				}
				if entry.Data.CyclesToCmdAvailable[key] < te.MinCycleInBetween {
					entry.Data.CyclesToCmdAvailable[key] = te.MinCycleInBetween
				}
			}
		}
	}
}

// recordActivateTimestamp records the current tick as an activate timestamp
// for the given rank and keeps only the last 4.
func recordActivateTimestamp(state *State, rank int) {
	history := findActivateHistory(&state.BankStates, rank)
	if history == nil {
		// Extend the histories slice if needed
		state.BankStates.ActivateHistories = append(
			state.BankStates.ActivateHistories,
			rankActivateHistory{Rank: rank},
		)
		history = &state.BankStates.ActivateHistories[len(state.BankStates.ActivateHistories)-1]
	}
	history.Timestamps = append(history.Timestamps, state.TickCount)
	// Keep only the last 4
	if len(history.Timestamps) > 4 {
		history.Timestamps = history.Timestamps[len(history.Timestamps)-4:]
	}
}

// cloneCommand creates a copy of a commandState with the same content.
func cloneCommand(cmd *commandState) *commandState {
	c := *cmd
	return &c
}
