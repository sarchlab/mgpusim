package l1v

import (
	"fmt"

	"gitlab.com/akita/akita"
)

func trace(now akita.VTimeInSec, where string, what string, addr uint64, data []byte) {
	s := ""
	s += fmt.Sprintf("%.15f, %s, %s, 0x%x", now, where, what, addr)
	if data != nil {
		s += ", ["
		for _, b := range data {
			s += fmt.Sprintf("%02x, ", b)
		}
		s += "]"
	}
	s += "\n"
	fmt.Print(s)
}
