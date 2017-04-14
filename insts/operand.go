package insts

import (
	"fmt"
	"math"
	"strings"
)

// Operand types
const (
	InvalidOperantType = iota
	RegOperand
	FloatOperand
	IntOperand
	LiteralConstant
)

// OperandType represents the type of an operand. It can be Reg, Float, Int ...
type OperandType int

// An Operand is an operand
type Operand struct {
	OperandType     OperandType
	Register        *Reg
	RegCount        int // for cases like v[0:3]
	FloatValue      float64
	IntValue        int64
	LiteralConstant uint64
}

// NewRegOperand returns a new operand of register type
func NewRegOperand(reg RegType, count int) *Operand {
	o := new(Operand)
	o.OperandType = RegOperand
	o.Register = Regs[reg]
	o.RegCount = count
	return o
}

// NewSRegOperand returns a new operand of s register type
func NewSRegOperand(index int, count int) *Operand {
	o := new(Operand)
	o.OperandType = RegOperand
	o.Register = Regs[S0+RegType(index)]
	o.RegCount = count
	return o
}

// NewVRegOperand returns a new operand of v register type
func NewVRegOperand(index int, count int) *Operand {
	o := new(Operand)
	o.OperandType = RegOperand
	o.Register = Regs[V0+RegType(index)]
	o.RegCount = count
	return o
}

// NewIntOperand returns a new operand of an integer type
func NewIntOperand(value int64) *Operand {
	o := new(Operand)
	o.OperandType = IntOperand
	o.IntValue = value
	return o
}

// NewFloatOperand returns a new operand of an floating point type
func NewFloatOperand(value float64) *Operand {
	o := new(Operand)
	o.OperandType = FloatOperand
	o.FloatValue = value
	return o
}

func (o Operand) String() string {
	switch o.OperandType {
	case RegOperand:
		if o.RegCount > 1 {
			if o.Register.IsSReg() {
				return fmt.Sprintf("s[%d:%d]",
					o.Register.RegIndex(), o.Register.RegIndex()+o.RegCount-1)
			} else if o.Register.IsVReg() {
				return fmt.Sprintf("v[%d:%d]",
					o.Register.RegIndex(), o.Register.RegIndex()+o.RegCount-1)
			} else if strings.Contains(o.Register.Name, "lo") {
				return o.Register.Name[:len(o.Register.Name)-2]
			}
			return fmt.Sprintf("<unknown: %+v>", o.Register)
		}
		return o.Register.Name
	case IntOperand:
		return fmt.Sprintf("%d", o.IntValue)
	case FloatOperand:
		return fmt.Sprintf("%f", o.FloatValue)
	case LiteralConstant:
		return "LiteralConstant"
	default:
		return ""
	}
}

func getOperand(num uint16) (*Operand, error) {
	switch {
	case num >= 0 && num <= 101:
		return NewSRegOperand(int(num), 0), nil

	case num == 102:
		return NewRegOperand(FlatSratchLo, 0), nil
	case num == 103:
		return NewRegOperand(FlatSratchHi, 0), nil
	case num == 104:
		return NewRegOperand(XnackMaskLo, 0), nil
	case num == 105:
		return NewRegOperand(XnackMaskHi, 0), nil
	case num == 106:
		return NewRegOperand(VccLo, 0), nil
	case num == 107:
		return NewRegOperand(VccHi, 0), nil
	case num == 108:
		return NewRegOperand(TbaLo, 0), nil
	case num == 109:
		return NewRegOperand(TbaHi, 0), nil
	case num == 110:
		return NewRegOperand(TmaLo, 0), nil
	case num == 111:
		return NewRegOperand(TmaHi, 0), nil

	case num >= 112 && num < 123:
		return NewRegOperand(Timp0+RegType(num-112), 0), nil

	case num == 124:
		return NewRegOperand(M0, 0), nil
	case num == 126:
		return NewRegOperand(ExecLo, 0), nil
	case num == 127:
		return NewRegOperand(ExecHi, 0), nil

	case num >= 128 && num <= 192:
		return NewIntOperand(int64(num) - 128), nil

	case num >= 193 && num <= 208:
		return NewIntOperand(192 - int64(num)), nil

	case num == 240:
		return NewFloatOperand(0.5), nil
	case num == 241:
		return NewFloatOperand(-0.5), nil
	case num == 242:
		return NewFloatOperand(1.0), nil
	case num == 243:
		return NewFloatOperand(-1.0), nil
	case num == 244:
		return NewFloatOperand(2.0), nil
	case num == 245:
		return NewFloatOperand(-2.0), nil
	case num == 247:
		return NewFloatOperand(4.0), nil
	case num == 247:
		return NewFloatOperand(-4.0), nil
	case num == 248:
		return NewFloatOperand(1.0 / (2.0 * math.Pi)), nil

	case num == 251:
		return NewRegOperand(Vccz, 0), nil
	case num == 252:
		return NewRegOperand(Execz, 0), nil
	case num == 253:
		return NewRegOperand(Scc, 0), nil

	case num == 255:
		return &Operand{LiteralConstant, nil, 0, 0, 0, 0}, nil

	case num >= 256 && num <= 511:
		return NewVRegOperand(int(num)-256, 0), nil

	default:
		return nil, fmt.Errorf("cannot find Operand %d", num)
	}
}
