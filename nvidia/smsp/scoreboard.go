package smsp

type Scoreboard struct {
	RegWriteBusy map[string]int
	RegReadBusy  map[string]int
}

func NewScoreboard() *Scoreboard {
	return &Scoreboard{
		RegWriteBusy: make(map[string]int),
		RegReadBusy:  make(map[string]int),
	}
}

func (s *Scoreboard) Reset() {
	s.RegWriteBusy = make(map[string]int)
	s.RegReadBusy = make(map[string]int)
}

func (s *Scoreboard) GetWriteBusy(reg string) bool {
	return s.RegWriteBusy[reg] > 0
}

func (s *Scoreboard) SetWriteBusy(reg string, busy bool) {
	if busy {
		if s.RegWriteBusy[reg] == 0 {
			s.RegWriteBusy[reg] = 1
		}
	} else {
		delete(s.RegWriteBusy, reg)
	}
}

func (s *Scoreboard) GetReadBusy(reg string) bool {
	return s.RegReadBusy[reg] > 0
}

func (s *Scoreboard) SetReadBusy(reg string, busy bool) {
	if busy {
		if s.RegReadBusy[reg] == 0 {
			s.RegReadBusy[reg] = 1
		}
	} else {
		delete(s.RegReadBusy, reg)
	}
}

func (s *Scoreboard) ClearReg(reg string) {
	delete(s.RegWriteBusy, reg)
	delete(s.RegReadBusy, reg)
}

func (s *Scoreboard) HasConflict(srcRegs []string, dstRegs []string) bool {
	for _, r := range srcRegs {
		if s.RegWriteBusy[r] > 0 {
			return true
		}
	}

	for _, r := range dstRegs {
		if s.RegWriteBusy[r] > 0 {
			return true
		}
	}

	for _, r := range dstRegs {
		if s.RegReadBusy[r] > 0 {
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
		s.RegReadBusy[r]++
	}
	for _, r := range dstRegs {
		s.RegWriteBusy[r]++
	}
}

func (s *Scoreboard) MarkSrcRegsConsumed(srcRegs []string) {
	for _, r := range srcRegs {
		if s.RegReadBusy[r] <= 1 {
			delete(s.RegReadBusy, r)
		} else {
			s.RegReadBusy[r]--
		}
	}
}

func (s *Scoreboard) MarkDstRegsCompleted(dstRegs []string) {
	for _, r := range dstRegs {
		if s.RegWriteBusy[r] <= 1 {
			delete(s.RegWriteBusy, r)
		} else {
			s.RegWriteBusy[r]--
		}
	}
}

func (s *Scoreboard) MarkCompleted(srcRegs []string, dstRegs []string) {
	s.MarkSrcRegsConsumed(srcRegs)
	s.MarkDstRegsCompleted(dstRegs)
}

func (s *Scoreboard) HasAnyBusy() bool {
	for _, v := range s.RegWriteBusy {
		if v > 0 {
			return true
		}
	}
	for _, v := range s.RegReadBusy {
		if v > 0 {
			return true
		}
	}
	return false
}
