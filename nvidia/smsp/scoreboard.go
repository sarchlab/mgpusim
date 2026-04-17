package smsp

type Scoreboard struct {
	regWriteBusy map[string]bool
	regReadBusy  map[string]bool
}

func NewScoreboard() *Scoreboard {
	return &Scoreboard{
		regWriteBusy: make(map[string]bool),
		regReadBusy:  make(map[string]bool),
	}
}

func (s *Scoreboard) Reset() {
	s.regWriteBusy = make(map[string]bool)
	s.regReadBusy = make(map[string]bool)
}

// Write

func (s *Scoreboard) GetWriteBusy(reg string) bool {
	return s.regWriteBusy[reg]
}

func (s *Scoreboard) SetWriteBusy(reg string, busy bool) {
	if busy {
		s.regWriteBusy[reg] = true
	} else {
		delete(s.regWriteBusy, reg)
	}
}

// Read

func (s *Scoreboard) GetReadBusy(reg string) bool {
	return s.regReadBusy[reg]
}

func (s *Scoreboard) SetReadBusy(reg string, busy bool) {
	if busy {
		s.regReadBusy[reg] = true
	} else {
		delete(s.regReadBusy, reg)
	}
}

// Clear all state for one register
func (s *Scoreboard) ClearReg(reg string) {
	delete(s.regWriteBusy, reg)
	delete(s.regReadBusy, reg)
}

// Check RAW or WAW hazard
// srcRegs = registers the inst reads
// dstRegs = registers the inst writes

func (s *Scoreboard) HasWriteConflict(srcRegs []string, dstRegs []string) bool {
	// return false
	// RAW: read-after-write
	for _, r := range srcRegs {
		if s.regWriteBusy[r] {
			return true
		}
	}

	// WAW: write-after-write
	for _, r := range dstRegs {
		if s.regWriteBusy[r] {
			return true
		}
	}

	return false
}

func (s *Scoreboard) HasReadConflict(srcRegs []string, dstRegs []string) bool {
	// return false
	// WAR: write-after-read
	for _, r := range dstRegs {
		if s.regReadBusy[r] {
			return true
		}
	}

	return false
}

func (s *Scoreboard) MarkIssued(srcRegs []string, dstRegs []string) {
	for _, r := range srcRegs {
		s.SetReadBusy(r, true)
	}
	for _, r := range dstRegs {
		s.SetWriteBusy(r, true)
	}
}

func (s *Scoreboard) MarkCompleted(srcRegs []string, dstRegs []string) {
	for _, r := range srcRegs {
		s.SetReadBusy(r, false)
	}
	for _, r := range dstRegs {
		s.SetWriteBusy(r, false)
	}
	// fmt.Printf("MarkCompleted called for srcRegs: %v, dstRegs: %v. Current write busy: %v, read busy: %v\n", srcRegs, dstRegs, s.regWriteBusy, s.regReadBusy)
}

func (s *Scoreboard) HasAnyBusy() bool {
	for _, v := range s.regWriteBusy {
		if v {
			return true
		}
	}
	for _, v := range s.regReadBusy {
		if v {
			return true
		}
	}
	return false
}

func (s *Scoreboard) getNumOfRegReadBusy() uint64 {
	count := uint64(0)
	for _, v := range s.regReadBusy {
		if v {
			count++
		}
	}
	return count
}

func (s *Scoreboard) getNumOfRegWriteBusy() uint64 {
	count := uint64(0)
	for _, v := range s.regWriteBusy {
		if v {
			count++
		}
	}
	return count
}
