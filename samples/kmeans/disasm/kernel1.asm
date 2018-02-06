	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	kmeans_kernel_compute   ; -- Begin function kmeans_kernel_compute
	.p2align	8
	.type	kmeans_kernel_compute,@function
	.amdgpu_hsa_kernel kmeans_kernel_compute
kmeans_kernel_compute:                  ; @kmeans_kernel_compute
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
		granulated_workitem_vgpr_count = 3
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
		kernarg_segment_byte_size = 80
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 14
		workitem_vgpr_count = 15
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
	s_load_dword s2, s[4:5], 0x4
	s_load_dword s5, s[6:7], 0x18
	s_load_dwordx2 s[0:1], s[6:7], 0x30
	s_waitcnt lgkmcnt(0)
	s_and_b32 s1, s2, 0xffff
	s_mul_i32 s8, s8, s1
	v_add_i32_e32 v0, vcc, s8, v0
	v_add_i32_e32 v0, vcc, s0, v0
	v_cmp_gt_u32_e32 vcc, s5, v0
	s_and_saveexec_b64 s[0:1], vcc
	; mask branch BB0_10
	s_cbranch_execz BB0_10
BB0_1:
	s_load_dwordx2 s[2:3], s[6:7], 0x10
	s_load_dword s4, s[6:7], 0x1c
	v_mov_b32_e32 v2, 0
	s_waitcnt lgkmcnt(0)
	s_cmp_lt_i32 s4, 1
	s_cbranch_scc1 BB0_9
; BB#2:
	s_load_dword s8, s[6:7], 0x20
	s_waitcnt lgkmcnt(0)
	s_cmp_gt_i32 s8, 0
	s_cbranch_scc0 BB0_7
; BB#3:                                 ; %.preheader
	s_load_dwordx2 s[10:11], s[6:7], 0x0
	s_load_dwordx2 s[0:1], s[6:7], 0x8
	s_ashr_i32 s9, s8, 31
	v_mov_b32_e32 v1, 0x7f7fffff
	s_lshl_b64 s[6:7], s[8:9], 2
	v_mov_b32_e32 v5, 0
	s_waitcnt lgkmcnt(0)
	v_mov_b32_e32 v4, s1
	v_mov_b32_e32 v3, s0
	v_mov_b32_e32 v2, 0
BB0_4:                                  ; =>This Loop Header: Depth=1
                                        ;     Child Loop BB0_5 Depth 2
	v_mov_b32_e32 v11, v4
	v_mov_b32_e32 v6, 0
	v_mov_b32_e32 v7, s8
	v_mov_b32_e32 v8, v0
	v_mov_b32_e32 v10, v3
BB0_5:                                  ;   Parent Loop BB0_4 Depth=1
                                        ; =>  This Inner Loop Header: Depth=2
	v_mov_b32_e32 v9, 0
	v_lshlrev_b64 v[12:13], 2, v[8:9]
	v_mov_b32_e32 v14, s11
	v_add_i32_e32 v12, vcc, s10, v12
	v_addc_u32_e32 v13, vcc, v14, v13, vcc
	flat_load_dword v9, v[12:13]
	v_readfirstlane_b32 s0, v10
	v_readfirstlane_b32 s1, v11
	s_load_dword s0, s[0:1], 0x0
	v_add_i32_e32 v10, vcc, 4, v10
	v_addc_u32_e32 v11, vcc, 0, v11, vcc
	v_add_i32_e32 v8, vcc, s5, v8
	v_add_i32_e32 v7, vcc, -1, v7
	v_cmp_ne_u32_e32 vcc, 0, v7
	s_and_b64 vcc, exec, vcc
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_subrev_f32_e32 v9, s0, v9
	v_mac_f32_e32 v6, v9, v9
	s_cbranch_vccnz BB0_5
; BB#6:                                 ;   in Loop: Header=BB0_4 Depth=1
	v_cmp_lt_f32_e64 s[0:1], v6, v1
	v_cndmask_b32_e64 v2, v2, v5, s[0:1]
	v_add_i32_e32 v5, vcc, 1, v5
	v_add_i32_e32 v3, vcc, s6, v3
	v_mov_b32_e32 v7, s7
	v_addc_u32_e32 v4, vcc, v4, v7, vcc
	v_cmp_eq_u32_e32 vcc, s4, v5
	s_and_b64 vcc, exec, vcc
	v_cndmask_b32_e64 v1, v1, v6, s[0:1]
	s_cbranch_vccz BB0_4
	s_branch BB0_9
BB0_7:                                  ; %.preheader13
	v_mov_b32_e32 v1, 0x7f7fffff
	v_mov_b32_e32 v3, 0
	v_mov_b32_e32 v2, 0
BB0_8:                                  ; =>This Inner Loop Header: Depth=1
	v_cmp_lt_f32_e64 s[0:1], 0, v1
	v_cndmask_b32_e64 v2, v2, v3, s[0:1]
	v_add_i32_e32 v3, vcc, 1, v3
	v_cmp_eq_u32_e32 vcc, s4, v3
	v_cndmask_b32_e64 v1, v1, 0, s[0:1]
	s_and_b64 vcc, exec, vcc
	s_cbranch_vccz BB0_8
BB0_9:                                  ; %.loopexit
	v_mov_b32_e32 v1, 0
	v_lshlrev_b64 v[0:1], 2, v[0:1]
	v_add_i32_e32 v0, vcc, s2, v0
	v_mov_b32_e32 v3, s3
	v_addc_u32_e32 v1, vcc, v3, v1, vcc
	flat_store_dword v[0:1], v2
BB0_10:
	s_endpgm
.Lfunc_end0:
	.size	kmeans_kernel_compute, .Lfunc_end0-kmeans_kernel_compute
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 420
; NumSgprs: 14
; NumVgprs: 15
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 1
; VGPRBlocks: 3
; NumSGPRsForWavesPerEU: 14
; NumVGPRsForWavesPerEU: 15
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
	.amdgpu_code_object_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            kmeans_kernel_compute
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            feature
        TypeName:        'float*'
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            clusters
        TypeName:        'float*'
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       I32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            membership
        TypeName:        'int*'
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
        Name:            npoints
        TypeName:        int
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
        Name:            nclusters
        TypeName:        int
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
        Name:            nfeatures
        TypeName:        int
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
        Name:            offset
        TypeName:        int
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
        Name:            size
        TypeName:        int
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
      KernargSegmentSize: 80
      WavefrontNumSGPRs: 14
      WorkitemNumVGPRs: 15
      KernargSegmentAlign: 4
      GroupSegmentAlign: 4
      PrivateSegmentAlign: 4
      WavefrontSize:   6
...
	.end_amdgpu_code_object_metadata
