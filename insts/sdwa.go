package insts

import "log"

// SDWASelect defines the sub-dword selection type
type SDWASelect uint32

// Defines all possible sub-dword selection type
const (
	SDWASelectByte0 SDWASelect = 0x000000ff
	SDWASelectByte1 SDWASelect = 0x0000ff00
	SDWASelectByte2 SDWASelect = 0x00ff0000
	SDWASelectByte3 SDWASelect = 0xff000000
	SDWASelectWord0 SDWASelect = 0x0000ffff
	SDWASelectWord1 SDWASelect = 0xffff0000
	SDWASelectDWord SDWASelect = 0xffffffff
)

// sdwaSelectString stringify SDWA select types
func sdwaSelectString(sdwaSelect SDWASelect) string {
	switch sdwaSelect {
	case SDWASelectByte0:
		return "BYTE_0"
	case SDWASelectByte1:
		return "BYTE_1"
	case SDWASelectByte2:
		return "BYTE_2"
	case SDWASelectByte3:
		return "BYTE_3"
	case SDWASelectWord0:
		return "WORD_0"
	case SDWASelectWord1:
		return "WORD_1"
	case SDWASelectDWord:
		return "DWORD"
	default:
		log.Panic("unknown SDWASelect type")
		return ""
	}
}

// SDWAUnused defines how to deal with unused bits
type SDWAUnused uint8

// Defines all possible SDWA unused options
const (
	SDWAUnusedPad      SDWAUnused = 0
	SDWAUnusedSEXT     SDWAUnused = 1
	SDWAUnusedPreserve SDWAUnused = 2
)

func sdwaUnusedString(sdwaUnused SDWAUnused) string {
	switch sdwaUnused {
	case SDWAUnusedPad:
		return "UNUSED_PAD"
	case SDWAUnusedSEXT:
		return "UNUSED_SEXT"
	case SDWAUnusedPreserve:
		return "UNUSED_PRESERVE"
	default:
		log.Panic("unknown SDWAUnused type")
		return ""
	}

}
