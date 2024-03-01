package nvidia

import (
	"fmt"
	"log"
)

type Register struct {
	rawText string
	regID   int32
	isZero  bool
}

func NewRegister(rawText string) *Register {
	reg, ok := registerTable[rawText]
	if !ok {
		reg = Register{rawText, -1, false}
		log.Panic("Unknown register: ", rawText)
	}

	return &reg
}

func (r *Register) String() string {
	return r.rawText
}

func (r *Register) ID() int32 {
	return r.regID
}

func (r *Register) IsZeroRegister() bool {
	return r.isZero
}

var registerTable map[string]Register

func init() {
	registerTable = make(map[string]Register)

	for i := 0; i < 32; i++ {
		registerTable[fmt.Sprintf("R%d", i)] = Register{fmt.Sprintf("R%d", i), int32(i), false}
	}

	registerTable["R255"] = Register{"R255", 255, true}
}
