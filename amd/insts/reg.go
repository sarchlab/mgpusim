package insts

// Reg is the representation of a register
type Reg struct {
	RegType  RegType
	Name     string
	ByteSize int
	IsBool   bool
}

// VReg returns a vector register object given a certain index
func VReg(index int) *Reg {
	return Regs[V0+RegType(index)]
}

// SReg returns a scalar register object given a certain index
func SReg(index int) *Reg {
	return Regs[S0+RegType(index)]
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

// RegType is the register type
type RegType int

// All the registers
const (
	InvalidRegType = iota
	PC
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
	EXEC
	EXECLO
	EXECHI
	EXECZ
	VCC
	VCCLO
	VCCHI
	VCCZ
	SCC
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
	VMCNT
	EXPCNT
	LGKMCNT
)

// Regs are a list of all registers
var Regs = map[RegType]*Reg{
	InvalidRegType: {InvalidRegType, "invalidregtype", 0, false},
	PC:             {PC, "pc", 8, false},
	V0:             {V0, "v0", 4, false},
	V1:             {V1, "v1", 4, false},
	V2:             {V2, "v2", 4, false},
	V3:             {V3, "v3", 4, false},
	V4:             {V4, "v4", 4, false},
	V5:             {V5, "v5", 4, false},
	V6:             {V6, "v6", 4, false},
	V7:             {V7, "v7", 4, false},
	V8:             {V8, "v8", 4, false},
	V9:             {V9, "v9", 4, false},
	V10:            {V10, "v10", 4, false},
	V11:            {V11, "v11", 4, false},
	V12:            {V12, "v12", 4, false},
	V13:            {V13, "v13", 4, false},
	V14:            {V14, "v14", 4, false},
	V15:            {V15, "v15", 4, false},
	V16:            {V16, "v16", 4, false},
	V17:            {V17, "v17", 4, false},
	V18:            {V18, "v18", 4, false},
	V19:            {V19, "v19", 4, false},
	V20:            {V20, "v20", 4, false},
	V21:            {V21, "v21", 4, false},
	V22:            {V22, "v22", 4, false},
	V23:            {V23, "v23", 4, false},
	V24:            {V24, "v24", 4, false},
	V25:            {V25, "v25", 4, false},
	V26:            {V26, "v26", 4, false},
	V27:            {V27, "v27", 4, false},
	V28:            {V28, "v28", 4, false},
	V29:            {V29, "v29", 4, false},
	V30:            {V30, "v30", 4, false},
	V31:            {V31, "v31", 4, false},
	V32:            {V32, "v32", 4, false},
	V33:            {V33, "v33", 4, false},
	V34:            {V34, "v34", 4, false},
	V35:            {V35, "v35", 4, false},
	V36:            {V36, "v36", 4, false},
	V37:            {V37, "v37", 4, false},
	V38:            {V38, "v38", 4, false},
	V39:            {V39, "v39", 4, false},
	V40:            {V40, "v40", 4, false},
	V41:            {V41, "v41", 4, false},
	V42:            {V42, "v42", 4, false},
	V43:            {V43, "v43", 4, false},
	V44:            {V44, "v44", 4, false},
	V45:            {V45, "v45", 4, false},
	V46:            {V46, "v46", 4, false},
	V47:            {V47, "v47", 4, false},
	V48:            {V48, "v48", 4, false},
	V49:            {V49, "v49", 4, false},
	V50:            {V50, "v50", 4, false},
	V51:            {V51, "v51", 4, false},
	V52:            {V52, "v52", 4, false},
	V53:            {V53, "v53", 4, false},
	V54:            {V54, "v54", 4, false},
	V55:            {V55, "v55", 4, false},
	V56:            {V56, "v56", 4, false},
	V57:            {V57, "v57", 4, false},
	V58:            {V58, "v58", 4, false},
	V59:            {V59, "v59", 4, false},
	V60:            {V60, "v60", 4, false},
	V61:            {V61, "v61", 4, false},
	V62:            {V62, "v62", 4, false},
	V63:            {V63, "v63", 4, false},
	V64:            {V64, "v64", 4, false},
	V65:            {V65, "v65", 4, false},
	V66:            {V66, "v66", 4, false},
	V67:            {V67, "v67", 4, false},
	V68:            {V68, "v68", 4, false},
	V69:            {V69, "v69", 4, false},
	V70:            {V70, "v70", 4, false},
	V71:            {V71, "v71", 4, false},
	V72:            {V72, "v72", 4, false},
	V73:            {V73, "v73", 4, false},
	V74:            {V74, "v74", 4, false},
	V75:            {V75, "v75", 4, false},
	V76:            {V76, "v76", 4, false},
	V77:            {V77, "v77", 4, false},
	V78:            {V78, "v78", 4, false},
	V79:            {V79, "v79", 4, false},
	V80:            {V80, "v80", 4, false},
	V81:            {V81, "v81", 4, false},
	V82:            {V82, "v82", 4, false},
	V83:            {V83, "v83", 4, false},
	V84:            {V84, "v84", 4, false},
	V85:            {V85, "v85", 4, false},
	V86:            {V86, "v86", 4, false},
	V87:            {V87, "v87", 4, false},
	V88:            {V88, "v88", 4, false},
	V89:            {V89, "v89", 4, false},
	V90:            {V90, "v90", 4, false},
	V91:            {V91, "v91", 4, false},
	V92:            {V92, "v92", 4, false},
	V93:            {V93, "v93", 4, false},
	V94:            {V94, "v94", 4, false},
	V95:            {V95, "v95", 4, false},
	V96:            {V96, "v96", 4, false},
	V97:            {V97, "v97", 4, false},
	V98:            {V98, "v98", 4, false},
	V99:            {V99, "v99", 4, false},
	V100:           {V100, "v100", 4, false},
	V101:           {V101, "v101", 4, false},
	V102:           {V102, "v102", 4, false},
	V103:           {V103, "v103", 4, false},
	V104:           {V104, "v104", 4, false},
	V105:           {V105, "v105", 4, false},
	V106:           {V106, "v106", 4, false},
	V107:           {V107, "v107", 4, false},
	V108:           {V108, "v108", 4, false},
	V109:           {V109, "v109", 4, false},
	V110:           {V110, "v110", 4, false},
	V111:           {V111, "v111", 4, false},
	V112:           {V112, "v112", 4, false},
	V113:           {V113, "v113", 4, false},
	V114:           {V114, "v114", 4, false},
	V115:           {V115, "v115", 4, false},
	V116:           {V116, "v116", 4, false},
	V117:           {V117, "v117", 4, false},
	V118:           {V118, "v118", 4, false},
	V119:           {V119, "v119", 4, false},
	V120:           {V120, "v120", 4, false},
	V121:           {V121, "v121", 4, false},
	V122:           {V122, "v122", 4, false},
	V123:           {V123, "v123", 4, false},
	V124:           {V124, "v124", 4, false},
	V125:           {V125, "v125", 4, false},
	V126:           {V126, "v126", 4, false},
	V127:           {V127, "v127", 4, false},
	V128:           {V128, "v128", 4, false},
	V129:           {V129, "v129", 4, false},
	V130:           {V130, "v130", 4, false},
	V131:           {V131, "v131", 4, false},
	V132:           {V132, "v132", 4, false},
	V133:           {V133, "v133", 4, false},
	V134:           {V134, "v134", 4, false},
	V135:           {V135, "v135", 4, false},
	V136:           {V136, "v136", 4, false},
	V137:           {V137, "v137", 4, false},
	V138:           {V138, "v138", 4, false},
	V139:           {V139, "v139", 4, false},
	V140:           {V140, "v140", 4, false},
	V141:           {V141, "v141", 4, false},
	V142:           {V142, "v142", 4, false},
	V143:           {V143, "v143", 4, false},
	V144:           {V144, "v144", 4, false},
	V145:           {V145, "v145", 4, false},
	V146:           {V146, "v146", 4, false},
	V147:           {V147, "v147", 4, false},
	V148:           {V148, "v148", 4, false},
	V149:           {V149, "v149", 4, false},
	V150:           {V150, "v150", 4, false},
	V151:           {V151, "v151", 4, false},
	V152:           {V152, "v152", 4, false},
	V153:           {V153, "v153", 4, false},
	V154:           {V154, "v154", 4, false},
	V155:           {V155, "v155", 4, false},
	V156:           {V156, "v156", 4, false},
	V157:           {V157, "v157", 4, false},
	V158:           {V158, "v158", 4, false},
	V159:           {V159, "v159", 4, false},
	V160:           {V160, "v160", 4, false},
	V161:           {V161, "v161", 4, false},
	V162:           {V162, "v162", 4, false},
	V163:           {V163, "v163", 4, false},
	V164:           {V164, "v164", 4, false},
	V165:           {V165, "v165", 4, false},
	V166:           {V166, "v166", 4, false},
	V167:           {V167, "v167", 4, false},
	V168:           {V168, "v168", 4, false},
	V169:           {V169, "v169", 4, false},
	V170:           {V170, "v170", 4, false},
	V171:           {V171, "v171", 4, false},
	V172:           {V172, "v172", 4, false},
	V173:           {V173, "v173", 4, false},
	V174:           {V174, "v174", 4, false},
	V175:           {V175, "v175", 4, false},
	V176:           {V176, "v176", 4, false},
	V177:           {V177, "v177", 4, false},
	V178:           {V178, "v178", 4, false},
	V179:           {V179, "v179", 4, false},
	V180:           {V180, "v180", 4, false},
	V181:           {V181, "v181", 4, false},
	V182:           {V182, "v182", 4, false},
	V183:           {V183, "v183", 4, false},
	V184:           {V184, "v184", 4, false},
	V185:           {V185, "v185", 4, false},
	V186:           {V186, "v186", 4, false},
	V187:           {V187, "v187", 4, false},
	V188:           {V188, "v188", 4, false},
	V189:           {V189, "v189", 4, false},
	V190:           {V190, "v190", 4, false},
	V191:           {V191, "v191", 4, false},
	V192:           {V192, "v192", 4, false},
	V193:           {V193, "v193", 4, false},
	V194:           {V194, "v194", 4, false},
	V195:           {V195, "v195", 4, false},
	V196:           {V196, "v196", 4, false},
	V197:           {V197, "v197", 4, false},
	V198:           {V198, "v198", 4, false},
	V199:           {V199, "v199", 4, false},
	V200:           {V200, "v200", 4, false},
	V201:           {V201, "v201", 4, false},
	V202:           {V202, "v202", 4, false},
	V203:           {V203, "v203", 4, false},
	V204:           {V204, "v204", 4, false},
	V205:           {V205, "v205", 4, false},
	V206:           {V206, "v206", 4, false},
	V207:           {V207, "v207", 4, false},
	V208:           {V208, "v208", 4, false},
	V209:           {V209, "v209", 4, false},
	V210:           {V210, "v210", 4, false},
	V211:           {V211, "v211", 4, false},
	V212:           {V212, "v212", 4, false},
	V213:           {V213, "v213", 4, false},
	V214:           {V214, "v214", 4, false},
	V215:           {V215, "v215", 4, false},
	V216:           {V216, "v216", 4, false},
	V217:           {V217, "v217", 4, false},
	V218:           {V218, "v218", 4, false},
	V219:           {V219, "v219", 4, false},
	V220:           {V220, "v220", 4, false},
	V221:           {V221, "v221", 4, false},
	V222:           {V222, "v222", 4, false},
	V223:           {V223, "v223", 4, false},
	V224:           {V224, "v224", 4, false},
	V225:           {V225, "v225", 4, false},
	V226:           {V226, "v226", 4, false},
	V227:           {V227, "v227", 4, false},
	V228:           {V228, "v228", 4, false},
	V229:           {V229, "v229", 4, false},
	V230:           {V230, "v230", 4, false},
	V231:           {V231, "v231", 4, false},
	V232:           {V232, "v232", 4, false},
	V233:           {V233, "v233", 4, false},
	V234:           {V234, "v234", 4, false},
	V235:           {V235, "v235", 4, false},
	V236:           {V236, "v236", 4, false},
	V237:           {V237, "v237", 4, false},
	V238:           {V238, "v238", 4, false},
	V239:           {V239, "v239", 4, false},
	V240:           {V240, "v240", 4, false},
	V241:           {V241, "v241", 4, false},
	V242:           {V242, "v242", 4, false},
	V243:           {V243, "v243", 4, false},
	V244:           {V244, "v244", 4, false},
	V245:           {V245, "v245", 4, false},
	V246:           {V246, "v246", 4, false},
	V247:           {V247, "v247", 4, false},
	V248:           {V248, "v248", 4, false},
	V249:           {V249, "v249", 4, false},
	V250:           {V250, "v250", 4, false},
	V251:           {V251, "v251", 4, false},
	V252:           {V252, "v252", 4, false},
	V253:           {V253, "v253", 4, false},
	V254:           {V254, "v254", 4, false},
	V255:           {V255, "v255", 4, false},
	S0:             {S0, "s0", 4, false},
	S1:             {S1, "s1", 4, false},
	S2:             {S2, "s2", 4, false},
	S3:             {S3, "s3", 4, false},
	S4:             {S4, "s4", 4, false},
	S5:             {S5, "s5", 4, false},
	S6:             {S6, "s6", 4, false},
	S7:             {S7, "s7", 4, false},
	S8:             {S8, "s8", 4, false},
	S9:             {S9, "s9", 4, false},
	S10:            {S10, "s10", 4, false},
	S11:            {S11, "s11", 4, false},
	S12:            {S12, "s12", 4, false},
	S13:            {S13, "s13", 4, false},
	S14:            {S14, "s14", 4, false},
	S15:            {S15, "s15", 4, false},
	S16:            {S16, "s16", 4, false},
	S17:            {S17, "s17", 4, false},
	S18:            {S18, "s18", 4, false},
	S19:            {S19, "s19", 4, false},
	S20:            {S20, "s20", 4, false},
	S21:            {S21, "s21", 4, false},
	S22:            {S22, "s22", 4, false},
	S23:            {S23, "s23", 4, false},
	S24:            {S24, "s24", 4, false},
	S25:            {S25, "s25", 4, false},
	S26:            {S26, "s26", 4, false},
	S27:            {S27, "s27", 4, false},
	S28:            {S28, "s28", 4, false},
	S29:            {S29, "s29", 4, false},
	S30:            {S30, "s30", 4, false},
	S31:            {S31, "s31", 4, false},
	S32:            {S32, "s32", 4, false},
	S33:            {S33, "s33", 4, false},
	S34:            {S34, "s34", 4, false},
	S35:            {S35, "s35", 4, false},
	S36:            {S36, "s36", 4, false},
	S37:            {S37, "s37", 4, false},
	S38:            {S38, "s38", 4, false},
	S39:            {S39, "s39", 4, false},
	S40:            {S40, "s40", 4, false},
	S41:            {S41, "s41", 4, false},
	S42:            {S42, "s42", 4, false},
	S43:            {S43, "s43", 4, false},
	S44:            {S44, "s44", 4, false},
	S45:            {S45, "s45", 4, false},
	S46:            {S46, "s46", 4, false},
	S47:            {S47, "s47", 4, false},
	S48:            {S48, "s48", 4, false},
	S49:            {S49, "s49", 4, false},
	S50:            {S50, "s50", 4, false},
	S51:            {S51, "s51", 4, false},
	S52:            {S52, "s52", 4, false},
	S53:            {S53, "s53", 4, false},
	S54:            {S54, "s54", 4, false},
	S55:            {S55, "s55", 4, false},
	S56:            {S56, "s56", 4, false},
	S57:            {S57, "s57", 4, false},
	S58:            {S58, "s58", 4, false},
	S59:            {S59, "s59", 4, false},
	S60:            {S60, "s60", 4, false},
	S61:            {S61, "s61", 4, false},
	S62:            {S62, "s62", 4, false},
	S63:            {S63, "s63", 4, false},
	S64:            {S64, "s64", 4, false},
	S65:            {S65, "s65", 4, false},
	S66:            {S66, "s66", 4, false},
	S67:            {S67, "s67", 4, false},
	S68:            {S68, "s68", 4, false},
	S69:            {S69, "s69", 4, false},
	S70:            {S70, "s70", 4, false},
	S71:            {S71, "s71", 4, false},
	S72:            {S72, "s72", 4, false},
	S73:            {S73, "s73", 4, false},
	S74:            {S74, "s74", 4, false},
	S75:            {S75, "s75", 4, false},
	S76:            {S76, "s76", 4, false},
	S77:            {S77, "s77", 4, false},
	S78:            {S78, "s78", 4, false},
	S79:            {S79, "s79", 4, false},
	S80:            {S80, "s80", 4, false},
	S81:            {S81, "s81", 4, false},
	S82:            {S82, "s82", 4, false},
	S83:            {S83, "s83", 4, false},
	S84:            {S84, "s84", 4, false},
	S85:            {S85, "s85", 4, false},
	S86:            {S86, "s86", 4, false},
	S87:            {S87, "s87", 4, false},
	S88:            {S88, "s88", 4, false},
	S89:            {S89, "s89", 4, false},
	S90:            {S90, "s90", 4, false},
	S91:            {S91, "s91", 4, false},
	S92:            {S92, "s92", 4, false},
	S93:            {S93, "s93", 4, false},
	S94:            {S94, "s94", 4, false},
	S95:            {S95, "s95", 4, false},
	S96:            {S96, "s96", 4, false},
	S97:            {S97, "s97", 4, false},
	S98:            {S98, "s98", 4, false},
	S99:            {S99, "s99", 4, false},
	S100:           {S100, "s100", 4, false},
	S101:           {S101, "s101", 4, false},
	EXEC:           {EXEC, "exec", 8, false},
	EXECLO:         {EXECLO, "execlo", 4, false},
	EXECHI:         {EXECHI, "exechi", 4, false},
	EXECZ:          {EXECZ, "execz", 1, true},
	VCC:            {VCC, "vcc", 8, false},
	VCCLO:          {VCCLO, "vcclo", 4, false},
	VCCHI:          {VCCHI, "vcchi", 4, false},
	VCCZ:           {VCCZ, "vccz", 1, true},
	SCC:            {SCC, "scc", 1, true},
	FlatSratch:     {FlatSratch, "flatsratch", 8, false},
	FlatSratchLo:   {FlatSratchLo, "flatsratchlo", 4, false},
	FlatSratchHi:   {FlatSratchHi, "flatsratchhi", 4, false},
	XnackMask:      {XnackMask, "xnackmask", 8, false},
	XnackMaskLo:    {XnackMaskLo, "xnackmasklo", 4, false},
	XnackMaskHi:    {XnackMaskHi, "xnackmaskhi", 4, false},
	Status:         {Status, "status", 4, false},
	Mode:           {Mode, "mode", 4, false},
	M0:             {M0, "m0", 4, false},
	Trapsts:        {Trapsts, "trapsts", 4, false},
	Tba:            {Tba, "tba", 8, false},
	TbaLo:          {TbaLo, "tbalo", 4, false},
	TbaHi:          {TbaHi, "tbahi", 4, false},
	Tma:            {Tma, "tma", 8, false},
	TmaLo:          {TmaLo, "tmalo", 4, false},
	TmaHi:          {TmaHi, "tmahi", 4, false},
	Timp0:          {Timp0, "timp0", 4, false},
	Timp1:          {Timp1, "timp1", 4, false},
	Timp2:          {Timp2, "timp2", 4, false},
	Timp3:          {Timp3, "timp3", 4, false},
	Timp4:          {Timp4, "timp4", 4, false},
	Timp5:          {Timp5, "timp5", 4, false},
	Timp6:          {Timp6, "timp6", 4, false},
	Timp7:          {Timp7, "timp7", 4, false},
	Timp8:          {Timp8, "timp8", 4, false},
	Timp9:          {Timp9, "timp9", 4, false},
	Timp10:         {Timp10, "timp10", 4, false},
	Timp11:         {Timp11, "timp11", 4, false},
	VMCNT:          {VMCNT, "vmcnt", 1, false},
	EXPCNT:         {EXPCNT, "expcnt", 1, false},
	LGKMCNT:        {LGKMCNT, "lgkmcnt", 1, false},
}
