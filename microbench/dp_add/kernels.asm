	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	microbench              ; -- Begin function microbench
	.p2align	8
	.type	microbench,@function
	.amdgpu_hsa_kernel microbench
microbench:                             ; @microbench
	.amd_kernel_code_t
		amd_code_version_major = 1
		amd_code_version_minor = 1
		amd_machine_kind = 1
		amd_machine_version_major = 8
		amd_machine_version_minor = 0
		amd_machine_version_stepping = 3
		kernel_code_entry_byte_offset = 256
		kernel_code_prefetch_byte_size = 0
		max_scratch_backing_memory_byte_size = 0
		granulated_workitem_vgpr_count = 2
		granulated_wavefront_sgpr_count = 1
		priority = 0
		float_mode = 192
		priv = 0
		enable_dx10_clamp = 1
		debug_mode = 0
		enable_ieee_mode = 1
		enable_sgpr_private_segment_wave_byte_offset = 0
		user_sgpr_count = 8
		enable_trap_handler = 1
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
		kernarg_segment_byte_size = 56
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 12
		workitem_vgpr_count = 12
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
; BB#0:
	s_load_dword s9, s[4:5], 0x4
	s_load_dwordx2 s[0:1], s[6:7], 0x0
	s_load_dwordx2 s[2:3], s[6:7], 0x8
	s_load_dwordx2 s[4:5], s[6:7], 0x10
	s_load_dword s6, s[6:7], 0x18
	s_waitcnt lgkmcnt(0)
	s_and_b32 s7, s9, 0xffff
	s_mul_i32 s8, s8, s7
	v_add_u32_e32 v0, vcc, s8, v0
	v_mov_b32_e32 v1, 0
	v_add_u32_e32 v0, vcc, s6, v0
	v_lshlrev_b64 v[4:5], 3, v[0:1]
	v_mov_b32_e32 v1, s5
	v_add_u32_e32 v0, vcc, s4, v4
	v_addc_u32_e32 v1, vcc, v1, v5, vcc
	v_add_u32_e32 v2, vcc, s0, v4
	v_mov_b32_e32 v3, s1
	v_addc_u32_e32 v3, vcc, v3, v5, vcc
	v_mov_b32_e32 v6, s3
	v_add_u32_e32 v4, vcc, s2, v4
	v_addc_u32_e32 v5, vcc, v6, v5, vcc
	flat_load_dwordx2 v[6:7], v[0:1]
	s_movk_i32 s0, 0x2710
BB0_1:                                  ; =>This Inner Loop Header: Depth=1
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_sub_i32 s0, s0, 20
	s_cmp_eq_u32 s0, 0
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	flat_load_dwordx2 v[8:9], v[4:5]
	flat_load_dwordx2 v[10:11], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_add_f64 v[6:7], v[6:7], v[10:11]
	v_add_f64 v[6:7], v[6:7], v[8:9]
	flat_store_dwordx2 v[0:1], v[6:7]
	s_cbranch_scc0 BB0_1
; BB#2:
	s_endpgm
.Lfunc_end0:
	.size	microbench, .Lfunc_end0-microbench
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 1020
; NumSgprs: 12
; NumVgprs: 12
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 1
; VGPRBlocks: 2
; NumSGPRsForWavesPerEU: 12
; NumVGPRsForWavesPerEU: 12
; ReservedVGPRFirst: 0
; ReservedVGPRCount: 0
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 1
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 0
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 0

	.ident	"clang version 4.0 "
	.section	".note.GNU-stack"
	.amd_amdgpu_isa "amdgcn-amd-amdhsa-amdgizcl-gfx803"
	.amd_amdgpu_hsa_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            microbench
    SymbolName:      'microbench@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            v1
        TypeName:        'double*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F64
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            v2
        TypeName:        'double*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F64
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            output
        TypeName:        'double*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F64
        AddrSpaceQual:   Global
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
    CodeProps:       
      KernargSegmentSize: 56
      GroupSegmentFixedSize: 0
      PrivateSegmentFixedSize: 0
      KernargSegmentAlign: 8
      WavefrontSize:   64
      NumSGPRs:        12
      NumVGPRs:        12
      MaxFlatWorkGroupSize: 256
...

	.end_amd_amdgpu_hsa_metadata
