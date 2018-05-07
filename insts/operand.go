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
	Code            int
	OperandType     OperandType
	Register        *Reg
	RegCount        int // for cases like v[0:3]
	FloatValue      float64
	IntValue        int64
	LiteralConstant uint32
}

// NewRegOperand returns a new operand of register type
func NewRegOperand(code int, reg RegType, count int) *Operand {
	o := new(Operand)
	o.Code = code
	o.OperandType = RegOperand
	o.Register = Regs[reg]
	o.RegCount = count
	return o
}

// NewSRegOperand returns a new operand of s register type
func NewSRegOperand(code int, index int, count int) *Operand {
	o := new(Operand)
	o.Code = code
	o.OperandType = RegOperand
	o.Register = Regs[S0+RegType(index)]
	o.RegCount = count
	return o
}

// NewVRegOperand returns a new operand of v register type
func NewVRegOperand(code int, index int, count int) *Operand {
	o := new(Operand)
	o.Code = code
	o.OperandType = RegOperand
	o.Register = Regs[V0+RegType(index)]
	o.RegCount = count
	return o
}

// NewIntOperand returns a new operand of an integer type
func NewIntOperand(code int, value int64) *Operand {
	o := new(Operand)
	o.Code = code
	o.OperandType = IntOperand
	o.IntValue = value
	return o
}

// NewFloatOperand returns a new operand of an floating point type
func NewFloatOperand(code int, value float64) *Operand {
	o := new(Operand)
	o.Code = code
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
		return fmt.Sprintf("0x%08x", o.LiteralConstant)
	default:
		return ""
	}
}

func getOperand(num uint16) (*Operand, error) {
	code := int(num)
	switch {
	case num >= 0 && num <= 101:
		return NewSRegOperand(code, code, 0), nil

	case num == 102:
		return NewRegOperand(code, FlatSratchLo, 0), nil
	case num == 103:
		return NewRegOperand(code, FlatSratchHi, 0), nil
	case num == 104:
		return NewRegOperand(code, XnackMaskLo, 0), nil
	case num == 105:
		return NewRegOperand(code, XnackMaskHi, 0), nil
	case num == 106:
		return NewRegOperand(code, VCCLO, 0), nil
	case num == 107:
		return NewRegOperand(code, VCCHI, 0), nil
	case num == 108:
		return NewRegOperand(code, TbaLo, 0), nil
	case num == 109:
		return NewRegOperand(code, TbaHi, 0), nil
	case num == 110:
		return NewRegOperand(code, TmaLo, 0), nil
	case num == 111:
		return NewRegOperand(code, TmaHi, 0), nil

	case num >= 112 && num < 123:
		return NewRegOperand(code, Timp0+RegType(num-112), 0), nil

	case num == 124:
		return NewRegOperand(code, M0, 0), nil
	case num == 126:
		return NewRegOperand(code, EXECLO, 0), nil
	case num == 127:
		return NewRegOperand(code, EXECHI, 0), nil

	case num >= 128 && num <= 192:
		return NewIntOperand(code, int64(num)-128), nil

	case num >= 193 && num <= 208:
		return NewIntOperand(code, 192-int64(num)), nil

	case num == 240:
		return NewFloatOperand(code, 0.5), nil
	case num == 241:
		return NewFloatOperand(code, -0.5), nil
	case num == 242:
		return NewFloatOperand(code, 1.0), nil
	case num == 243:
		return NewFloatOperand(code, -1.0), nil
	case num == 244:
		return NewFloatOperand(code, 2.0), nil
	case num == 245:
		return NewFloatOperand(code, -2.0), nil
	case num == 246:
		return NewFloatOperand(code, 4.0), nil
	case num == 247:
		return NewFloatOperand(code, -4.0), nil
	case num == 248:
		return NewFloatOperand(code, 1.0/(2.0*math.Pi)), nil
	case num == 251:
		return NewRegOperand(code, VCCZ, 0), nil
	case num == 252:
		return NewRegOperand(code, EXECZ, 0), nil
	case num == 253:
		return NewRegOperand(code, SCC, 0), nil

	case num == 255:
		return &Operand{code, LiteralConstant, nil, 0, 0, 0, 0}, nil

	case num >= 256 && num <= 511:
		return NewVRegOperand(code, int(num)-256, 0), nil

	default:
		return nil, fmt.Errorf("cannot find Operand %d", num)
	}
}
