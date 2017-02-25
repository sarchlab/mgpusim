package disasm

import (
	"fmt"
	"math"
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

// Reg is the representation of a register
type Reg struct {
	RegType  RegType
	Name     string
	ByteSize int
	IsBool   bool
}

// IsVReg checks if a register is a vector register
func (r *Reg) IsVReg() bool {
	return r.RegType >= V0 && r.RegType <= V255
}

// IsSReg checks if a register is a scalar register
func (r *Reg) IsSReg() bool {
	return r.RegType >= S0 && r.RegType <= S101
}

// RegIndex returns the index of the index in the s-series or the v-series.
// If the register is not s or v register, -1 is returned.
func (r *Reg) RegIndex() int {
	if r.IsSReg() {
		return int(r.RegType - S0)
	} else if r.IsVReg() {
		return int(r.RegType - V0)
	}
	return -1
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

// An Instruction is a GCN3 instructino
type Instruction struct {
	*Format
	*InstType
	ByteSize int

	Src0 *Operand
	Src1 *Operand
	Dst  *Operand

	Addr   *Operand
	Data   *Operand
	Base   *Operand
	Offset *Operand

	SystemLevelCoherent bool
	GlobalLevelCoherent bool
	TextureFailEnable   bool
	Imm                 bool
}

func (i Instruction) sop2String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String() + ", " +
		i.Src1.String()
}

func (i Instruction) vop1String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String()
}

func (i Instruction) flatString() string {
	var s string
	if i.Opcode >= 16 && i.Opcode <= 23 {
		s = i.InstName + " " + i.Dst.String() + ", " +
			i.Addr.String()
	} else if i.Opcode >= 24 && i.Opcode <= 31 {
		s = i.InstName + " " + i.Addr.String() + ", " +
			i.Dst.String()
	}
	return s
}

func (i Instruction) smemString() string {
	// TODO: Consider store instructions, and the case if imm = 0
	s := fmt.Sprintf("%s %s, %s, %#x",
		i.InstName, i.Data.String(), i.Base.String(), uint16(i.Offset.IntValue))
	return s
}

func (i Instruction) String() string {
	switch i.FormatType {
	case sop2:
		return i.sop2String()
	case smem:
		return i.smemString()
	case vop1:
		return i.vop1String()
	case flat:
		return i.flatString()
	default:
		return i.InstName
	}
}

// RegType is the register type
type RegType int

// All the registers
const (
	InvalidRegType = iota
	Pc
	V0
	V1
	V2
	V3
	V4
	V5
	V6
	V7
	V8
	V9
	V10
	V11
	V12
	V13
	V14
	V15
	V16
	V17
	V18
	V19
	V20
	V21
	V22
	V23
	V24
	V25
	V26
	V27
	V28
	V29
	V30
	V31
	V32
	V33
	V34
	V35
	V36
	V37
	V38
	V39
	V40
	V41
	V42
	V43
	V44
	V45
	V46
	V47
	V48
	V49
	V50
	V51
	V52
	V53
	V54
	V55
	V56
	V57
	V58
	V59
	V60
	V61
	V62
	V63
	V64
	V65
	V66
	V67
	V68
	V69
	V70
	V71
	V72
	V73
	V74
	V75
	V76
	V77
	V78
	V79
	V80
	V81
	V82
	V83
	V84
	V85
	V86
	V87
	V88
	V89
	V90
	V91
	V92
	V93
	V94
	V95
	V96
	V97
	V98
	V99
	V100
	V101
	V102
	V103
	V104
	V105
	V106
	V107
	V108
	V109
	V110
	V111
	V112
	V113
	V114
	V115
	V116
	V117
	V118
	V119
	V120
	V121
	V122
	V123
	V124
	V125
	V126
	V127
	V128
	V129
	V130
	V131
	V132
	V133
	V134
	V135
	V136
	V137
	V138
	V139
	V140
	V141
	V142
	V143
	V144
	V145
	V146
	V147
	V148
	V149
	V150
	V151
	V152
	V153
	V154
	V155
	V156
	V157
	V158
	V159
	V160
	V161
	V162
	V163
	V164
	V165
	V166
	V167
	V168
	V169
	V170
	V171
	V172
	V173
	V174
	V175
	V176
	V177
	V178
	V179
	V180
	V181
	V182
	V183
	V184
	V185
	V186
	V187
	V188
	V189
	V190
	V191
	V192
	V193
	V194
	V195
	V196
	V197
	V198
	V199
	V200
	V201
	V202
	V203
	V204
	V205
	V206
	V207
	V208
	V209
	V210
	V211
	V212
	V213
	V214
	V215
	V216
	V217
	V218
	V219
	V220
	V221
	V222
	V223
	V224
	V225
	V226
	V227
	V228
	V229
	V230
	V231
	V232
	V233
	V234
	V235
	V236
	V237
	V238
	V239
	V240
	V241
	V242
	V243
	V244
	V245
	V246
	V247
	V248
	V249
	V250
	V251
	V252
	V253
	V254
	V255
	S0
	S1
	S2
	S3
	S4
	S5
	S6
	S7
	S8
	S9
	S10
	S11
	S12
	S13
	S14
	S15
	S16
	S17
	S18
	S19
	S20
	S21
	S22
	S23
	S24
	S25
	S26
	S27
	S28
	S29
	S30
	S31
	S32
	S33
	S34
	S35
	S36
	S37
	S38
	S39
	S40
	S41
	S42
	S43
	S44
	S45
	S46
	S47
	S48
	S49
	S50
	S51
	S52
	S53
	S54
	S55
	S56
	S57
	S58
	S59
	S60
	S61
	S62
	S63
	S64
	S65
	S66
	S67
	S68
	S69
	S70
	S71
	S72
	S73
	S74
	S75
	S76
	S77
	S78
	S79
	S80
	S81
	S82
	S83
	S84
	S85
	S86
	S87
	S88
	S89
	S90
	S91
	S92
	S93
	S94
	S95
	S96
	S97
	S98
	S99
	S100
	S101
	Exec
	ExecLo
	ExecHi
	Execz
	Vcc
	VccLo
	VccHi
	Vccz
	Scc
	FlatSratch
	FlatSratchLo
	FlatSratchHi
	XnackMask
	XnackMaskLo
	XnackMaskHi
	Status
	Mode
	M0
	Trapsts
	Tba
	TbaLo
	TbaHi
	Tma
	TmaLo
	TmaHi
	Timp0
	Timp1
	Timp2
	Timp3
	Timp4
	Timp5
	Timp6
	Timp7
	Timp8
	Timp9
	Timp10
	Timp11
	Vmcnt
	Expcnt
	Lgkmcnt
)

// Regs are a list of all registers
var Regs = map[RegType]*Reg{
	InvalidRegType: &Reg{InvalidRegType, "invalidregtype", 0, false},
	Pc:             &Reg{Pc, "pc", 8, false},
	V0:             &Reg{V0, "v0", 4, false},
	V1:             &Reg{V1, "v1", 4, false},
	V2:             &Reg{V2, "v2", 4, false},
	V3:             &Reg{V3, "v3", 4, false},
	V4:             &Reg{V4, "v4", 4, false},
	V5:             &Reg{V5, "v5", 4, false},
	V6:             &Reg{V6, "v6", 4, false},
	V7:             &Reg{V7, "v7", 4, false},
	V8:             &Reg{V8, "v8", 4, false},
	V9:             &Reg{V9, "v9", 4, false},
	V10:            &Reg{V10, "v10", 4, false},
	V11:            &Reg{V11, "v11", 4, false},
	V12:            &Reg{V12, "v12", 4, false},
	V13:            &Reg{V13, "v13", 4, false},
	V14:            &Reg{V14, "v14", 4, false},
	V15:            &Reg{V15, "v15", 4, false},
	V16:            &Reg{V16, "v16", 4, false},
	V17:            &Reg{V17, "v17", 4, false},
	V18:            &Reg{V18, "v18", 4, false},
	V19:            &Reg{V19, "v19", 4, false},
	V20:            &Reg{V20, "v20", 4, false},
	V21:            &Reg{V21, "v21", 4, false},
	V22:            &Reg{V22, "v22", 4, false},
	V23:            &Reg{V23, "v23", 4, false},
	V24:            &Reg{V24, "v24", 4, false},
	V25:            &Reg{V25, "v25", 4, false},
	V26:            &Reg{V26, "v26", 4, false},
	V27:            &Reg{V27, "v27", 4, false},
	V28:            &Reg{V28, "v28", 4, false},
	V29:            &Reg{V29, "v29", 4, false},
	V30:            &Reg{V30, "v30", 4, false},
	V31:            &Reg{V31, "v31", 4, false},
	V32:            &Reg{V32, "v32", 4, false},
	V33:            &Reg{V33, "v33", 4, false},
	V34:            &Reg{V34, "v34", 4, false},
	V35:            &Reg{V35, "v35", 4, false},
	V36:            &Reg{V36, "v36", 4, false},
	V37:            &Reg{V37, "v37", 4, false},
	V38:            &Reg{V38, "v38", 4, false},
	V39:            &Reg{V39, "v39", 4, false},
	V40:            &Reg{V40, "v40", 4, false},
	V41:            &Reg{V41, "v41", 4, false},
	V42:            &Reg{V42, "v42", 4, false},
	V43:            &Reg{V43, "v43", 4, false},
	V44:            &Reg{V44, "v44", 4, false},
	V45:            &Reg{V45, "v45", 4, false},
	V46:            &Reg{V46, "v46", 4, false},
	V47:            &Reg{V47, "v47", 4, false},
	V48:            &Reg{V48, "v48", 4, false},
	V49:            &Reg{V49, "v49", 4, false},
	V50:            &Reg{V50, "v50", 4, false},
	V51:            &Reg{V51, "v51", 4, false},
	V52:            &Reg{V52, "v52", 4, false},
	V53:            &Reg{V53, "v53", 4, false},
	V54:            &Reg{V54, "v54", 4, false},
	V55:            &Reg{V55, "v55", 4, false},
	V56:            &Reg{V56, "v56", 4, false},
	V57:            &Reg{V57, "v57", 4, false},
	V58:            &Reg{V58, "v58", 4, false},
	V59:            &Reg{V59, "v59", 4, false},
	V60:            &Reg{V60, "v60", 4, false},
	V61:            &Reg{V61, "v61", 4, false},
	V62:            &Reg{V62, "v62", 4, false},
	V63:            &Reg{V63, "v63", 4, false},
	V64:            &Reg{V64, "v64", 4, false},
	V65:            &Reg{V65, "v65", 4, false},
	V66:            &Reg{V66, "v66", 4, false},
	V67:            &Reg{V67, "v67", 4, false},
	V68:            &Reg{V68, "v68", 4, false},
	V69:            &Reg{V69, "v69", 4, false},
	V70:            &Reg{V70, "v70", 4, false},
	V71:            &Reg{V71, "v71", 4, false},
	V72:            &Reg{V72, "v72", 4, false},
	V73:            &Reg{V73, "v73", 4, false},
	V74:            &Reg{V74, "v74", 4, false},
	V75:            &Reg{V75, "v75", 4, false},
	V76:            &Reg{V76, "v76", 4, false},
	V77:            &Reg{V77, "v77", 4, false},
	V78:            &Reg{V78, "v78", 4, false},
	V79:            &Reg{V79, "v79", 4, false},
	V80:            &Reg{V80, "v80", 4, false},
	V81:            &Reg{V81, "v81", 4, false},
	V82:            &Reg{V82, "v82", 4, false},
	V83:            &Reg{V83, "v83", 4, false},
	V84:            &Reg{V84, "v84", 4, false},
	V85:            &Reg{V85, "v85", 4, false},
	V86:            &Reg{V86, "v86", 4, false},
	V87:            &Reg{V87, "v87", 4, false},
	V88:            &Reg{V88, "v88", 4, false},
	V89:            &Reg{V89, "v89", 4, false},
	V90:            &Reg{V90, "v90", 4, false},
	V91:            &Reg{V91, "v91", 4, false},
	V92:            &Reg{V92, "v92", 4, false},
	V93:            &Reg{V93, "v93", 4, false},
	V94:            &Reg{V94, "v94", 4, false},
	V95:            &Reg{V95, "v95", 4, false},
	V96:            &Reg{V96, "v96", 4, false},
	V97:            &Reg{V97, "v97", 4, false},
	V98:            &Reg{V98, "v98", 4, false},
	V99:            &Reg{V99, "v99", 4, false},
	V100:           &Reg{V100, "v100", 4, false},
	V101:           &Reg{V101, "v101", 4, false},
	V102:           &Reg{V102, "v102", 4, false},
	V103:           &Reg{V103, "v103", 4, false},
	V104:           &Reg{V104, "v104", 4, false},
	V105:           &Reg{V105, "v105", 4, false},
	V106:           &Reg{V106, "v106", 4, false},
	V107:           &Reg{V107, "v107", 4, false},
	V108:           &Reg{V108, "v108", 4, false},
	V109:           &Reg{V109, "v109", 4, false},
	V110:           &Reg{V110, "v110", 4, false},
	V111:           &Reg{V111, "v111", 4, false},
	V112:           &Reg{V112, "v112", 4, false},
	V113:           &Reg{V113, "v113", 4, false},
	V114:           &Reg{V114, "v114", 4, false},
	V115:           &Reg{V115, "v115", 4, false},
	V116:           &Reg{V116, "v116", 4, false},
	V117:           &Reg{V117, "v117", 4, false},
	V118:           &Reg{V118, "v118", 4, false},
	V119:           &Reg{V119, "v119", 4, false},
	V120:           &Reg{V120, "v120", 4, false},
	V121:           &Reg{V121, "v121", 4, false},
	V122:           &Reg{V122, "v122", 4, false},
	V123:           &Reg{V123, "v123", 4, false},
	V124:           &Reg{V124, "v124", 4, false},
	V125:           &Reg{V125, "v125", 4, false},
	V126:           &Reg{V126, "v126", 4, false},
	V127:           &Reg{V127, "v127", 4, false},
	V128:           &Reg{V128, "v128", 4, false},
	V129:           &Reg{V129, "v129", 4, false},
	V130:           &Reg{V130, "v130", 4, false},
	V131:           &Reg{V131, "v131", 4, false},
	V132:           &Reg{V132, "v132", 4, false},
	V133:           &Reg{V133, "v133", 4, false},
	V134:           &Reg{V134, "v134", 4, false},
	V135:           &Reg{V135, "v135", 4, false},
	V136:           &Reg{V136, "v136", 4, false},
	V137:           &Reg{V137, "v137", 4, false},
	V138:           &Reg{V138, "v138", 4, false},
	V139:           &Reg{V139, "v139", 4, false},
	V140:           &Reg{V140, "v140", 4, false},
	V141:           &Reg{V141, "v141", 4, false},
	V142:           &Reg{V142, "v142", 4, false},
	V143:           &Reg{V143, "v143", 4, false},
	V144:           &Reg{V144, "v144", 4, false},
	V145:           &Reg{V145, "v145", 4, false},
	V146:           &Reg{V146, "v146", 4, false},
	V147:           &Reg{V147, "v147", 4, false},
	V148:           &Reg{V148, "v148", 4, false},
	V149:           &Reg{V149, "v149", 4, false},
	V150:           &Reg{V150, "v150", 4, false},
	V151:           &Reg{V151, "v151", 4, false},
	V152:           &Reg{V152, "v152", 4, false},
	V153:           &Reg{V153, "v153", 4, false},
	V154:           &Reg{V154, "v154", 4, false},
	V155:           &Reg{V155, "v155", 4, false},
	V156:           &Reg{V156, "v156", 4, false},
	V157:           &Reg{V157, "v157", 4, false},
	V158:           &Reg{V158, "v158", 4, false},
	V159:           &Reg{V159, "v159", 4, false},
	V160:           &Reg{V160, "v160", 4, false},
	V161:           &Reg{V161, "v161", 4, false},
	V162:           &Reg{V162, "v162", 4, false},
	V163:           &Reg{V163, "v163", 4, false},
	V164:           &Reg{V164, "v164", 4, false},
	V165:           &Reg{V165, "v165", 4, false},
	V166:           &Reg{V166, "v166", 4, false},
	V167:           &Reg{V167, "v167", 4, false},
	V168:           &Reg{V168, "v168", 4, false},
	V169:           &Reg{V169, "v169", 4, false},
	V170:           &Reg{V170, "v170", 4, false},
	V171:           &Reg{V171, "v171", 4, false},
	V172:           &Reg{V172, "v172", 4, false},
	V173:           &Reg{V173, "v173", 4, false},
	V174:           &Reg{V174, "v174", 4, false},
	V175:           &Reg{V175, "v175", 4, false},
	V176:           &Reg{V176, "v176", 4, false},
	V177:           &Reg{V177, "v177", 4, false},
	V178:           &Reg{V178, "v178", 4, false},
	V179:           &Reg{V179, "v179", 4, false},
	V180:           &Reg{V180, "v180", 4, false},
	V181:           &Reg{V181, "v181", 4, false},
	V182:           &Reg{V182, "v182", 4, false},
	V183:           &Reg{V183, "v183", 4, false},
	V184:           &Reg{V184, "v184", 4, false},
	V185:           &Reg{V185, "v185", 4, false},
	V186:           &Reg{V186, "v186", 4, false},
	V187:           &Reg{V187, "v187", 4, false},
	V188:           &Reg{V188, "v188", 4, false},
	V189:           &Reg{V189, "v189", 4, false},
	V190:           &Reg{V190, "v190", 4, false},
	V191:           &Reg{V191, "v191", 4, false},
	V192:           &Reg{V192, "v192", 4, false},
	V193:           &Reg{V193, "v193", 4, false},
	V194:           &Reg{V194, "v194", 4, false},
	V195:           &Reg{V195, "v195", 4, false},
	V196:           &Reg{V196, "v196", 4, false},
	V197:           &Reg{V197, "v197", 4, false},
	V198:           &Reg{V198, "v198", 4, false},
	V199:           &Reg{V199, "v199", 4, false},
	V200:           &Reg{V200, "v200", 4, false},
	V201:           &Reg{V201, "v201", 4, false},
	V202:           &Reg{V202, "v202", 4, false},
	V203:           &Reg{V203, "v203", 4, false},
	V204:           &Reg{V204, "v204", 4, false},
	V205:           &Reg{V205, "v205", 4, false},
	V206:           &Reg{V206, "v206", 4, false},
	V207:           &Reg{V207, "v207", 4, false},
	V208:           &Reg{V208, "v208", 4, false},
	V209:           &Reg{V209, "v209", 4, false},
	V210:           &Reg{V210, "v210", 4, false},
	V211:           &Reg{V211, "v211", 4, false},
	V212:           &Reg{V212, "v212", 4, false},
	V213:           &Reg{V213, "v213", 4, false},
	V214:           &Reg{V214, "v214", 4, false},
	V215:           &Reg{V215, "v215", 4, false},
	V216:           &Reg{V216, "v216", 4, false},
	V217:           &Reg{V217, "v217", 4, false},
	V218:           &Reg{V218, "v218", 4, false},
	V219:           &Reg{V219, "v219", 4, false},
	V220:           &Reg{V220, "v220", 4, false},
	V221:           &Reg{V221, "v221", 4, false},
	V222:           &Reg{V222, "v222", 4, false},
	V223:           &Reg{V223, "v223", 4, false},
	V224:           &Reg{V224, "v224", 4, false},
	V225:           &Reg{V225, "v225", 4, false},
	V226:           &Reg{V226, "v226", 4, false},
	V227:           &Reg{V227, "v227", 4, false},
	V228:           &Reg{V228, "v228", 4, false},
	V229:           &Reg{V229, "v229", 4, false},
	V230:           &Reg{V230, "v230", 4, false},
	V231:           &Reg{V231, "v231", 4, false},
	V232:           &Reg{V232, "v232", 4, false},
	V233:           &Reg{V233, "v233", 4, false},
	V234:           &Reg{V234, "v234", 4, false},
	V235:           &Reg{V235, "v235", 4, false},
	V236:           &Reg{V236, "v236", 4, false},
	V237:           &Reg{V237, "v237", 4, false},
	V238:           &Reg{V238, "v238", 4, false},
	V239:           &Reg{V239, "v239", 4, false},
	V240:           &Reg{V240, "v240", 4, false},
	V241:           &Reg{V241, "v241", 4, false},
	V242:           &Reg{V242, "v242", 4, false},
	V243:           &Reg{V243, "v243", 4, false},
	V244:           &Reg{V244, "v244", 4, false},
	V245:           &Reg{V245, "v245", 4, false},
	V246:           &Reg{V246, "v246", 4, false},
	V247:           &Reg{V247, "v247", 4, false},
	V248:           &Reg{V248, "v248", 4, false},
	V249:           &Reg{V249, "v249", 4, false},
	V250:           &Reg{V250, "v250", 4, false},
	V251:           &Reg{V251, "v251", 4, false},
	V252:           &Reg{V252, "v252", 4, false},
	V253:           &Reg{V253, "v253", 4, false},
	V254:           &Reg{V254, "v254", 4, false},
	V255:           &Reg{V255, "v255", 4, false},
	S0:             &Reg{S0, "s0", 4, false},
	S1:             &Reg{S1, "s1", 4, false},
	S2:             &Reg{S2, "s2", 4, false},
	S3:             &Reg{S3, "s3", 4, false},
	S4:             &Reg{S4, "s4", 4, false},
	S5:             &Reg{S5, "s5", 4, false},
	S6:             &Reg{S6, "s6", 4, false},
	S7:             &Reg{S7, "s7", 4, false},
	S8:             &Reg{S8, "s8", 4, false},
	S9:             &Reg{S9, "s9", 4, false},
	S10:            &Reg{S10, "s10", 4, false},
	S11:            &Reg{S11, "s11", 4, false},
	S12:            &Reg{S12, "s12", 4, false},
	S13:            &Reg{S13, "s13", 4, false},
	S14:            &Reg{S14, "s14", 4, false},
	S15:            &Reg{S15, "s15", 4, false},
	S16:            &Reg{S16, "s16", 4, false},
	S17:            &Reg{S17, "s17", 4, false},
	S18:            &Reg{S18, "s18", 4, false},
	S19:            &Reg{S19, "s19", 4, false},
	S20:            &Reg{S20, "s20", 4, false},
	S21:            &Reg{S21, "s21", 4, false},
	S22:            &Reg{S22, "s22", 4, false},
	S23:            &Reg{S23, "s23", 4, false},
	S24:            &Reg{S24, "s24", 4, false},
	S25:            &Reg{S25, "s25", 4, false},
	S26:            &Reg{S26, "s26", 4, false},
	S27:            &Reg{S27, "s27", 4, false},
	S28:            &Reg{S28, "s28", 4, false},
	S29:            &Reg{S29, "s29", 4, false},
	S30:            &Reg{S30, "s30", 4, false},
	S31:            &Reg{S31, "s31", 4, false},
	S32:            &Reg{S32, "s32", 4, false},
	S33:            &Reg{S33, "s33", 4, false},
	S34:            &Reg{S34, "s34", 4, false},
	S35:            &Reg{S35, "s35", 4, false},
	S36:            &Reg{S36, "s36", 4, false},
	S37:            &Reg{S37, "s37", 4, false},
	S38:            &Reg{S38, "s38", 4, false},
	S39:            &Reg{S39, "s39", 4, false},
	S40:            &Reg{S40, "s40", 4, false},
	S41:            &Reg{S41, "s41", 4, false},
	S42:            &Reg{S42, "s42", 4, false},
	S43:            &Reg{S43, "s43", 4, false},
	S44:            &Reg{S44, "s44", 4, false},
	S45:            &Reg{S45, "s45", 4, false},
	S46:            &Reg{S46, "s46", 4, false},
	S47:            &Reg{S47, "s47", 4, false},
	S48:            &Reg{S48, "s48", 4, false},
	S49:            &Reg{S49, "s49", 4, false},
	S50:            &Reg{S50, "s50", 4, false},
	S51:            &Reg{S51, "s51", 4, false},
	S52:            &Reg{S52, "s52", 4, false},
	S53:            &Reg{S53, "s53", 4, false},
	S54:            &Reg{S54, "s54", 4, false},
	S55:            &Reg{S55, "s55", 4, false},
	S56:            &Reg{S56, "s56", 4, false},
	S57:            &Reg{S57, "s57", 4, false},
	S58:            &Reg{S58, "s58", 4, false},
	S59:            &Reg{S59, "s59", 4, false},
	S60:            &Reg{S60, "s60", 4, false},
	S61:            &Reg{S61, "s61", 4, false},
	S62:            &Reg{S62, "s62", 4, false},
	S63:            &Reg{S63, "s63", 4, false},
	S64:            &Reg{S64, "s64", 4, false},
	S65:            &Reg{S65, "s65", 4, false},
	S66:            &Reg{S66, "s66", 4, false},
	S67:            &Reg{S67, "s67", 4, false},
	S68:            &Reg{S68, "s68", 4, false},
	S69:            &Reg{S69, "s69", 4, false},
	S70:            &Reg{S70, "s70", 4, false},
	S71:            &Reg{S71, "s71", 4, false},
	S72:            &Reg{S72, "s72", 4, false},
	S73:            &Reg{S73, "s73", 4, false},
	S74:            &Reg{S74, "s74", 4, false},
	S75:            &Reg{S75, "s75", 4, false},
	S76:            &Reg{S76, "s76", 4, false},
	S77:            &Reg{S77, "s77", 4, false},
	S78:            &Reg{S78, "s78", 4, false},
	S79:            &Reg{S79, "s79", 4, false},
	S80:            &Reg{S80, "s80", 4, false},
	S81:            &Reg{S81, "s81", 4, false},
	S82:            &Reg{S82, "s82", 4, false},
	S83:            &Reg{S83, "s83", 4, false},
	S84:            &Reg{S84, "s84", 4, false},
	S85:            &Reg{S85, "s85", 4, false},
	S86:            &Reg{S86, "s86", 4, false},
	S87:            &Reg{S87, "s87", 4, false},
	S88:            &Reg{S88, "s88", 4, false},
	S89:            &Reg{S89, "s89", 4, false},
	S90:            &Reg{S90, "s90", 4, false},
	S91:            &Reg{S91, "s91", 4, false},
	S92:            &Reg{S92, "s92", 4, false},
	S93:            &Reg{S93, "s93", 4, false},
	S94:            &Reg{S94, "s94", 4, false},
	S95:            &Reg{S95, "s95", 4, false},
	S96:            &Reg{S96, "s96", 4, false},
	S97:            &Reg{S97, "s97", 4, false},
	S98:            &Reg{S98, "s98", 4, false},
	S99:            &Reg{S99, "s99", 4, false},
	S100:           &Reg{S100, "s100", 4, false},
	S101:           &Reg{S101, "s101", 4, false},
	Exec:           &Reg{Exec, "exec", 8, false},
	ExecLo:         &Reg{ExecLo, "execlo", 4, false},
	ExecHi:         &Reg{ExecHi, "exechi", 4, false},
	Execz:          &Reg{Execz, "execz", 1, true},
	Vcc:            &Reg{Vcc, "vcc", 8, false},
	VccLo:          &Reg{VccLo, "vcclo", 4, false},
	VccHi:          &Reg{VccHi, "vcchi", 4, false},
	Vccz:           &Reg{Vccz, "vccz", 1, true},
	Scc:            &Reg{Scc, "scc", 1, true},
	FlatSratch:     &Reg{FlatSratch, "flatsratch", 8, false},
	FlatSratchLo:   &Reg{FlatSratchLo, "flatsratchlo", 4, false},
	FlatSratchHi:   &Reg{FlatSratchHi, "flatsratchhi", 4, false},
	XnackMask:      &Reg{XnackMask, "xnackmask", 8, false},
	XnackMaskLo:    &Reg{XnackMaskLo, "xnackmasklo", 4, false},
	XnackMaskHi:    &Reg{XnackMaskHi, "xnackmaskhi", 4, false},
	Status:         &Reg{Status, "status", 4, false},
	Mode:           &Reg{Mode, "mode", 4, false},
	M0:             &Reg{M0, "m0", 4, false},
	Trapsts:        &Reg{Trapsts, "trapsts", 4, false},
	Tba:            &Reg{Tba, "tba", 8, false},
	TbaLo:          &Reg{TbaLo, "tbalo", 4, false},
	TbaHi:          &Reg{TbaHi, "tbahi", 4, false},
	Tma:            &Reg{Tma, "tma", 8, false},
	TmaLo:          &Reg{TmaLo, "tmalo", 4, false},
	TmaHi:          &Reg{TmaHi, "tmahi", 4, false},
	Timp0:          &Reg{Timp0, "timp0", 4, false},
	Timp1:          &Reg{Timp1, "timp1", 4, false},
	Timp2:          &Reg{Timp2, "timp2", 4, false},
	Timp3:          &Reg{Timp3, "timp3", 4, false},
	Timp4:          &Reg{Timp4, "timp4", 4, false},
	Timp5:          &Reg{Timp5, "timp5", 4, false},
	Timp6:          &Reg{Timp6, "timp6", 4, false},
	Timp7:          &Reg{Timp7, "timp7", 4, false},
	Timp8:          &Reg{Timp8, "timp8", 4, false},
	Timp9:          &Reg{Timp9, "timp9", 4, false},
	Timp10:         &Reg{Timp10, "timp10", 4, false},
	Timp11:         &Reg{Timp11, "timp11", 4, false},
	Vmcnt:          &Reg{Vmcnt, "vmcnt", 1, false},
	Expcnt:         &Reg{Expcnt, "expcnt", 1, false},
	Lgkmcnt:        &Reg{Lgkmcnt, "lgkmcnt", 1, false},
}
