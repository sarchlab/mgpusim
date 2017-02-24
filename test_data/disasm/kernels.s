s_add_u32 s0, s4, 4
s_addc_u32 s1, s5, 0
v_mov_b32_e32 v3, s0
v_mov_b32_e32 v4, s1
flat_load_ushort v1, v[3:4]
s_load_dwordx2 s[0:1], s[6:7], 0x28
s_load_dword s9, s[6:7], 0x20
s_load_dwordx2 s[10:11], s[6:7], 0x0
s_load_dwordx2 s[12:13], s[6:7], 0x8
s_load_dwordx2 s[2:3], s[6:7], 0x18
s_load_dword s4, s[4:5], 0xc
s_waitcnt lgkmcnt(0)
v_mov_b32_e32 v3, s1
v_mov_b32_e32 v2, 0
s_cmp_eq_u32 s9, 0
s_waitcnt vmcnt(0)
v_mul_lo_u32 v1, v1, s8
v_add_i32_e32 v0, vcc, v0, v1
v_add_i32_e32 v0, vcc, s0, v0
v_addc_u32_e32 v1, vcc, 0, v3, vcc
s_cbranch_scc1 41
s_load_dwordx2 s[0:1], s[6:7], 0x10
v_mov_b32_e32 v2, 0
v_mov_b32_e32 v1, 0
v_mov_b32_e32 v5, v0
s_waitcnt lgkmcnt(0)
v_mov_b32_e32 v4, s1
v_mov_b32_e32 v3, s0
v_cmp_lt_u32_e64 s[0:1], v0, v1
v_mov_b32_e32 v6, s9
v_cndmask_b32_e64 v6, 0, v6, s[0:1]
v_add_i32_e32 v6, vcc, v6, v5
v_mov_b32_e32 v7, 0
v_mov_b32_e32 v8, s10
v_mov_b32_e32 v9, s2
v_mov_b32_e32 v10, s11
v_mov_b32_e32 v11, s3
v_cndmask_b32_e64 v8, v8, v9, s[0:1]
v_lshlrev_b64 v[6:7], 2, v[6:7]
v_add_i32_e32 v8, vcc, v8, v6
v_cndmask_b32_e64 v9, v10, v11, s[0:1]
v_addc_u32_e32 v9, vcc, v9, v7, vcc
flat_load_dword v6, v[3:4]
s_nop 0
flat_load_dword v7, v[8:9]
v_add_i32_e32 v1, vcc, 1, v1
v_add_i32_e32 v5, vcc, -1, v5
v_add_i32_e32 v3, vcc, 4, v3
v_addc_u32_e32 v4, vcc, 0, v4, vcc
v_cmp_ne_u32_e32 vcc, s9, v1
s_and_b64 vcc, exec, vcc
s_waitcnt vmcnt(0) lgkmcnt(0)
v_mac_f32_e32 v2, v7, v6
s_cbranch_vccnz 65503
v_mov_b32_e32 v1, 0
v_lshlrev_b64 v[3:4], 2, v[0:1]
v_add_i32_e32 v5, vcc, s12, v3
v_mov_b32_e32 v3, s13
v_addc_u32_e32 v6, vcc, v4, v3, vcc
flat_store_dword v[5:6], v2
s_waitcnt vmcnt(0) lgkmcnt(0)
s_barrier
s_sub_i32 s0, s4, s9
v_cmp_le_u32_e32 vcc, s0, v0
s_and_saveexec_b64 s[0:1], vcc
s_xor_b64 s[0:1], exec, s[0:1]
s_cbranch_execz 19
v_lshlrev_b64 v[1:2], 2, v[0:1]
v_add_i32_e32 v3, vcc, s10, v1
v_mov_b32_e32 v1, s11
s_sub_i32 s4, s9, s4
v_addc_u32_e32 v4, vcc, v2, v1, vcc
v_add_i32_e32 v0, vcc, s4, v0
v_mov_b32_e32 v1, 0
v_lshlrev_b64 v[0:1], 2, v[0:1]
v_add_i32_e32 v5, vcc, s2, v0
v_mov_b32_e32 v0, s3
v_addc_u32_e32 v6, vcc, v1, v0, vcc
flat_load_dword v0, v[3:4]
s_waitcnt vmcnt(0) lgkmcnt(0)
flat_store_dword v[5:6], v0
s_waitcnt vmcnt(0) lgkmcnt(0)
s_or_b64 exec, exec, s[0:1]
s_barrier
s_endpgm
