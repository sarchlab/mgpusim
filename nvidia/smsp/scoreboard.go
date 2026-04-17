package smsp

type Scoreboard struct {
	regWriteBusy map[string]int
	regReadBusy  map[string]int
}

func NewScoreboard() *Scoreboard {
	return &Scoreboard{
		regWriteBusy: make(map[string]int),
		regReadBusy:  make(map[string]int),
	}
}

func (s *Scoreboard) Reset() {
	s.regWriteBusy = make(map[string]int)
	s.regReadBusy = make(map[string]int)
}

func (s *Scoreboard) GetWriteBusy(reg string) bool {
	return s.regWriteBusy[reg] > 0
}

func (s *Scoreboard) SetWriteBusy(reg string, busy bool) {
	if busy {
		if s.regWriteBusy[reg] == 0 {
			s.regWriteBusy[reg] = 1
		}
	} else {
		delete(s.regWriteBusy, reg)
	}
}

func (s *Scoreboard) GetReadBusy(reg string) bool {
	return s.regReadBusy[reg] > 0
}

func (s *Scoreboard) SetReadBusy(reg string, busy bool) {
	if busy {
		if s.regReadBusy[reg] == 0 {
			s.regReadBusy[reg] = 1
		}
	} else {
		delete(s.regReadBusy, reg)
	}
}

func (s *Scoreboard) ClearReg(reg string) {
	delete(s.regWriteBusy, reg)
	delete(s.regReadBusy, reg)
}

func (s *Scoreboard) HasConflict(srcRegs []string, dstRegs []string) bool {
	for _, r := range srcRegs {
		if s.regWriteBusy[r] > 0 {
			return true
		}
	}

	for _, r := range dstRegs {
		if s.regWriteBusy[r] > 0 {
			return true
		}
	}

	for _, r := range dstRegs {
		if s.regReadBusy[r] > 0 {
			return true
		}
	}

	return false
}

func (s *Scoreboard) HasWriteConflict(srcRegs []string, dstRegs []string) bool {
	return s.HasConflict(srcRegs, dstRegs)
}

func (s *Scoreboard) HasReadConflict(srcRegs []string, dstRegs []string) bool {
	return s.HasConflict(srcRegs, dstRegs)
}

func (s *Scoreboard) MarkIssued(srcRegs []string, dstRegs []string) {
	for _, r := range srcRegs {
		s.regReadBusy[r]++
	}
	for _, r := range dstRegs {
		s.regWriteBusy[r]++
	}
}

func (s *Scoreboard) MarkSrcRegsConsumed(srcRegs []string) {
	for _, r := range srcRegs {
		if s.regReadBusy[r] <= 1 {
			delete(s.regReadBusy, r)
		} else {
			s.regReadBusy[r]--
		}
	}
}

func (s *Scoreboard) MarkDstRegsCompleted(dstRegs []string) {
	for _, r := range dstRegs {
		if s.regWriteBusy[r] <= 1 {
			delete(s.regWriteBusy, r)
		} else {
			s.regWriteBusy[r]--
		}
	}
}

func (s *Scoreboard) MarkCompleted(srcRegs []string, dstRegs []string) {
	s.MarkSrcRegsConsumed(srcRegs)
	s.MarkDstRegsCompleted(dstRegs)
}

func (s *Scoreboard) HasAnyBusy() bool {
	for _, v := range s.regWriteBusy {
		if v > 0 {
			return true
		}
	}
	for _, v := range s.regReadBusy {
		if v > 0 {
			return true
		}
	}
	return false
}

func (s *Scoreboard) getNumOfRegReadBusy() uint64 {
	count := uint64(0)
	for _, v := range s.regReadBusy {
		if v > 0 {
			count++
		}
	}
	return count
}

func (s *Scoreboard) getNumOfRegWriteBusy() uint64 {
	count := uint64(0)
	for _, v := range s.regWriteBusy {
		if v > 0 {
			count++
		}
	}
	return count
}
