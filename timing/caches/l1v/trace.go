package l1v

import (
	"gitlab.com/akita/akita"
)

func trace(now akita.VTimeInSec, what string, addr uint64, data []byte) {
	// s := ""
	// s += fmt.Sprintf("%.15f, %s, 0x%x", now, what, addr)
	// if data != nil {
	// 	s += ", ["
	// 	for _, b := range data {
	// 		s += fmt.Sprintf("%02x, ", b)
	// 	}
	// 	s += "]"
	// }
	// s += "\n"
	// fmt.Print(s)
}
