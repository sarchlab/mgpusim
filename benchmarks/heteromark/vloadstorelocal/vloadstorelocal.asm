	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.hidden	vlsl                    ; -- Begin function vlsl
	.globl	vlsl
	.p2align	8
	.type	vlsl,@function
	.amdgpu_hsa_kernel vlsl
vlsl:                                   ; @vlsl
	.amd_kernel_code_t
		amd_code_version_major = 1
		amd_code_version_minor = 2
		amd_machine_kind = 1
		amd_machine_version_major = 8
		amd_machine_version_minor = 0
		amd_machine_version_stepping = 3
		kernel_code_entry_byte_offset = 256
		kernel_code_prefetch_byte_size = 0
		granulated_workitem_vgpr_count = 5
		granulated_wavefront_sgpr_count = 1
		priority = 0
		float_mode = 192
		priv = 0
		enable_dx10_clamp = 1
		debug_mode = 0
		enable_ieee_mode = 1
		enable_sgpr_private_segment_wave_byte_offset = 0
		user_sgpr_count = 8
		enable_trap_handler = 0
		enable_sgpr_workgroup_id_x = 1
		enable_sgpr_workgroup_id_y = 0
		enable_sgpr_workgroup_id_z = 0
		enable_sgpr_workgroup_info = 0
		enable_vgpr_workitem_id = 0
		enable_exception_msb = 0
		granulated_lds_size = 0
		enable_exception = 0
		enable_sgpr_private_segment_buffer = 1
		enable_sgpr_dispatch_ptr = 1
		enable_sgpr_queue_ptr = 0
		enable_sgpr_kernarg_segment_ptr = 1
		enable_sgpr_dispatch_id = 0
		enable_sgpr_flat_scratch_init = 0
		enable_sgpr_private_segment_size = 0
		enable_sgpr_grid_workgroup_count_x = 0
		enable_sgpr_grid_workgroup_count_y = 0
		enable_sgpr_grid_workgroup_count_z = 0
		enable_ordered_append_gds = 0
		private_element_size = 1
		is_ptr64 = 1
		is_dynamic_callstack = 0
		is_debug_enabled = 0
		is_xnack_enabled = 0
		workitem_private_segment_byte_size = 0
		workgroup_group_segment_byte_size = 0
		gds_segment_byte_size = 0
		kernarg_segment_byte_size = 72
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 11
		workitem_vgpr_count = 21
		reserved_vgpr_first = 0
		reserved_vgpr_count = 0
		reserved_sgpr_first = 0
		reserved_sgpr_count = 0
		debug_wavefront_private_segment_offset_sgpr = 0
		debug_private_segment_buffer_sgpr = 0
		kernarg_segment_alignment = 4
		group_segment_alignment = 4
		private_segment_alignment = 4
		wavefront_size = 6
		call_convention = -1
		runtime_loader_kernel_symbol = 0
	.end_amd_kernel_code_t
; %bb.0:
	s_load_dword s0, s[4:5], 0x4
	s_load_dwordx2 s[4:5], s[6:7], 0x10
	s_load_dword s1, s[6:7], 0x18
	s_waitcnt lgkmcnt(0)
	s_and_b32 s0, s0, 0xffff
	s_mul_i32 s8, s8, s0
	v_add_u32_e32 v1, vcc, s8, v0
	v_add_u32_e32 v1, vcc, s1, v1
	v_lshlrev_b32_e32 v1, 4, v1
	v_cmp_gt_u32_e32 vcc, s5, v1
	s_and_saveexec_b64 s[0:1], vcc
	; mask branch BB0_2
	s_cbranch_execz BB0_2
BB0_1:
	s_load_dwordx4 s[0:3], s[6:7], 0x0
	v_ashrrev_i32_e32 v2, 31, v1
	v_lshlrev_b64 v[18:19], 2, v[1:2]
	v_lshlrev_b32_e32 v0, 6, v0
	s_mov_b32 m0, -1
	s_waitcnt lgkmcnt(0)
	v_mov_b32_e32 v2, s1
	v_add_u32_e32 v10, vcc, s0, v18
	v_addc_u32_e32 v11, vcc, v2, v19, vcc
	v_add_u32_e32 v6, vcc, 48, v10
	v_addc_u32_e32 v7, vcc, 0, v11, vcc
	v_add_u32_e32 v2, vcc, 32, v10
	v_addc_u32_e32 v3, vcc, 0, v11, vcc
	v_add_u32_e32 v14, vcc, 16, v10
	v_addc_u32_e32 v15, vcc, 0, v11, vcc
	flat_load_dwordx4 v[10:13], v[10:11]
	flat_load_dwordx4 v[14:17], v[14:15]
	flat_load_dwordx4 v[2:5], v[2:3]
	flat_load_dwordx4 v[6:9], v[6:7]
	v_add_u32_e32 v20, vcc, s4, v0
	v_or_b32_e32 v0, 4, v1
	s_waitcnt vmcnt(3) lgkmcnt(3)
	ds_write2_b32 v20, v12, v13 offset0:2 offset1:3
	ds_write2_b32 v20, v10, v11 offset1:1
	v_or_b32_e32 v10, 8, v1
	v_or_b32_e32 v12, 12, v1
	v_ashrrev_i32_e32 v1, 31, v0
	s_waitcnt vmcnt(2) lgkmcnt(4)
	ds_write2_b32 v20, v16, v17 offset0:6 offset1:7
	ds_write2_b32 v20, v14, v15 offset0:4 offset1:5
	v_lshlrev_b64 v[0:1], 2, v[0:1]
	v_mov_b32_e32 v15, s3
	v_add_u32_e32 v14, vcc, s2, v18
	v_ashrrev_i32_e32 v11, 31, v10
	v_addc_u32_e32 v15, vcc, v15, v19, vcc
	v_add_u32_e32 v16, vcc, s2, v0
	v_mov_b32_e32 v17, s3
	v_lshlrev_b64 v[10:11], 2, v[10:11]
	v_ashrrev_i32_e32 v13, 31, v12
	v_addc_u32_e32 v17, vcc, v17, v1, vcc
	v_lshlrev_b64 v[12:13], 2, v[12:13]
	v_mov_b32_e32 v0, s3
	v_add_u32_e32 v10, vcc, s2, v10
	v_addc_u32_e32 v11, vcc, v0, v11, vcc
	v_add_u32_e32 v12, vcc, s2, v12
	s_waitcnt vmcnt(1) lgkmcnt(5)
	ds_write2_b32 v20, v4, v5 offset0:10 offset1:11
	ds_write2_b32 v20, v2, v3 offset0:8 offset1:9
	s_waitcnt vmcnt(0) lgkmcnt(6)
	ds_write2_b32 v20, v8, v9 offset0:14 offset1:15
	ds_write2_b32 v20, v6, v7 offset0:12 offset1:13
	s_waitcnt lgkmcnt(0)
	s_barrier
	v_addc_u32_e32 v13, vcc, v0, v13, vcc
	ds_read2_b32 v[0:1], v20 offset1:1
	ds_read2_b32 v[2:3], v20 offset0:2 offset1:3
	s_waitcnt lgkmcnt(0)
	flat_store_dwordx4 v[14:15], v[0:3]
	ds_read2_b32 v[0:1], v20 offset0:4 offset1:5
	ds_read2_b32 v[2:3], v20 offset0:6 offset1:7
	s_waitcnt lgkmcnt(0)
	flat_store_dwordx4 v[16:17], v[0:3]
	ds_read2_b32 v[0:1], v20 offset0:8 offset1:9
	ds_read2_b32 v[2:3], v20 offset0:10 offset1:11
	s_waitcnt lgkmcnt(0)
	flat_store_dwordx4 v[10:11], v[0:3]
	ds_read2_b32 v[0:1], v20 offset0:12 offset1:13
	ds_read2_b32 v[2:3], v20 offset0:14 offset1:15
	s_waitcnt lgkmcnt(0)
	flat_store_dwordx4 v[12:13], v[0:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	s_barrier
BB0_2:
	s_endpgm
.Lfunc_end0:
	.size	vlsl, .Lfunc_end0-vlsl
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 472
; NumSgprs: 11
; NumVgprs: 21
; ScratchSize: 0
; MemoryBound: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 1
; VGPRBlocks: 5
; NumSGPRsForWavesPerEU: 11
; NumVGPRsForWavesPerEU: 21
; WaveLimiterHint : 1
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 0
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 0
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 0

	.ident	"clang version 8.0 "
	.section	".note.GNU-stack"
	.addrsig
	.amd_amdgpu_isa "amdgcn-amd-amdhsa-amdgizcl-gfx803"
	.amd_amdgpu_hsa_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            vlsl
    SymbolName:      'vlsl@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            a
        TypeName:        'int*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       I32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            c
        TypeName:        'int*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       I32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            a_tmp
        TypeName:        'int*'
        Size:            4
        Align:           4
        ValueKind:       DynamicSharedPointer
        ValueType:       I32
        PointeeAlign:    4
        AddrSpaceQual:   Local
        AccQual:         Default
      - Name:            count
        TypeName:        uint
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       U32
        AccQual:         Default
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetX
        ValueType:       I64
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetY
        ValueType:       I64
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetZ
        ValueType:       I64
      - Size:            8
        Align:           8
        ValueKind:       HiddenNone
        ValueType:       I8
        AddrSpaceQual:   Global
      - Size:            8
        Align:           8
        ValueKind:       HiddenNone
        ValueType:       I8
        AddrSpaceQual:   Global
      - Size:            8
        Align:           8
        ValueKind:       HiddenNone
        ValueType:       I8
        AddrSpaceQual:   Global
    CodeProps:       
      KernargSegmentSize: 72
      GroupSegmentFixedSize: 0
      PrivateSegmentFixedSize: 0
      KernargSegmentAlign: 8
      WavefrontSize:   64
      NumSGPRs:        11
      NumVGPRs:        21
      MaxFlatWorkGroupSize: 256
...

	.end_amd_amdgpu_hsa_metadata
