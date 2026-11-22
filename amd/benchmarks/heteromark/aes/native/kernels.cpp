/**
 * kernels.cpp: HIP implementation of AES encryption kernel
 * Translated from kernels.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

typedef unsigned char uchar;
typedef unsigned int uint;

__device__ void SubBytes(uchar* input, uchar *s) {
  input[0] = s[input[0]];
  input[1] = s[input[1]];
  input[2] = s[input[2]];
  input[3] = s[input[3]];
  input[4] = s[input[4]];
  input[5] = s[input[5]];
  input[6] = s[input[6]];
  input[7] = s[input[7]];
  input[8] = s[input[8]];
  input[9] = s[input[9]];
  input[10] = s[input[10]];
  input[11] = s[input[11]];
  input[12] = s[input[12]];
  input[13] = s[input[13]];
  input[14] = s[input[14]];
  input[15] = s[input[15]];
}

__device__ void MixColumns(uchar* arr) {
  for (int i = 0; i < 4; i++) {
    uchar a[4];
    uchar b[4];
    uchar c;
    uchar h;
    for (c = 0; c < 4; c++) {
      a[c] = arr[(4 * i + c)];
      h = a[c] & 0x80;
      b[c] = a[c] << 1;
      if (h == 0x80) {
        b[c] ^= 0x1b;
      }
    }
    arr[i * 4 + 0] = b[0] ^ a[3] ^ a[2] ^ b[1] ^ a[1];
    arr[i * 4 + 1] = b[1] ^ a[0] ^ a[3] ^ b[2] ^ a[2];
    arr[i * 4 + 2] = b[2] ^ a[1] ^ a[0] ^ b[3] ^ a[3];
    arr[i * 4 + 3] = b[3] ^ a[2] ^ a[1] ^ b[0] ^ a[0];
  }
}

__device__ void ShiftRows(uchar* input) {
  uchar state[16];
  state[0] = input[0];
  state[1] = input[5];
  state[2] = input[10];
  state[3] = input[15];
  state[4] = input[4];
  state[5] = input[9];
  state[6] = input[14];
  state[7] = input[3];
  state[8] = input[8];
  state[9] = input[13];
  state[10] = input[2];
  state[11] = input[7];
  state[12] = input[12];
  state[13] = input[1];
  state[14] = input[6];
  state[15] = input[11];

  input[0] = state[0];
  input[1] = state[1];
  input[2] = state[2];
  input[3] = state[3];
  input[4] = state[4];
  input[5] = state[5];
  input[6] = state[6];
  input[7] = state[7];
  input[8] = state[8];
  input[9] = state[9];
  input[10] = state[10];
  input[11] = state[11];
  input[12] = state[12];
  input[13] = state[13];
  input[14] = state[14];
  input[15] = state[15];
}

__device__ void AddRoundKey(uchar* state, uint* expanded_key, int offset) {
  for (int i = 0; i < 4; i++) {
    uint word = expanded_key[offset + i];
    uchar bytes[4];

    bytes[0] = (word & 0xff000000) >> 24;
    bytes[1] = (word & 0x00ff0000) >> 16;
    bytes[2] = (word & 0x0000ff00) >> 8;
    bytes[3] = (word & 0x000000ff) >> 0;

    state[i * 4] ^= bytes[0];
    state[i * 4 + 1] ^= bytes[1];
    state[i * 4 + 2] ^= bytes[2];
    state[i * 4 + 3] ^= bytes[3];
  }
}

extern "C" __global__ void Encrypt(uchar* input, 
                                    uint* expanded_key, 
                                    uchar* s) {
  uchar state[16];

  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  for (int i = 0; i < 16; i++) {
    state[i] = input[tid * 16 + i];
  }

  AddRoundKey(state, expanded_key, 0);

  for (int i = 1; i < 14; i++) {
    SubBytes(state, s);
    ShiftRows(state);
    MixColumns(state);
    AddRoundKey(state, expanded_key, i * 4);
  }

  SubBytes(state, s);
  ShiftRows(state);
  AddRoundKey(state, expanded_key, 14 * 4);

  for (int i = 0; i < 16; i++) {
    input[tid * 16 + i] = state[i];
  }
}


// GPU Benchmarks v1.1, heteromark_aes/hip/heteromark_aes.hip.
__constant__ static const unsigned char d_sbox[256] = {
    0x63,0x7c,0x77,0x7b,0xf2,0x6b,0x6f,0xc5,0x30,0x01,0x67,0x2b,0xfe,0xd7,0xab,0x76,
    0xca,0x82,0xc9,0x7d,0xfa,0x59,0x47,0xf0,0xad,0xd4,0xa2,0xaf,0x9c,0xa4,0x72,0xc0,
    0xb7,0xfd,0x93,0x26,0x36,0x3f,0xf7,0xcc,0x34,0xa5,0xe5,0xf1,0x71,0xd8,0x31,0x15,
    0x04,0xc7,0x23,0xc3,0x18,0x96,0x05,0x9a,0x07,0x12,0x80,0xe2,0xeb,0x27,0xb2,0x75,
    0x09,0x83,0x2c,0x1a,0x1b,0x6e,0x5a,0xa0,0x52,0x3b,0xd6,0xb3,0x29,0xe3,0x2f,0x84,
    0x53,0xd1,0x00,0xed,0x20,0xfc,0xb1,0x5b,0x6a,0xcb,0xbe,0x39,0x4a,0x4c,0x58,0xcf,
    0xd0,0xef,0xaa,0xfb,0x43,0x4d,0x33,0x85,0x45,0xf9,0x02,0x7f,0x50,0x3c,0x9f,0xa8,
    0x51,0xa3,0x40,0x8f,0x92,0x9d,0x38,0xf5,0xbc,0xb6,0xda,0x21,0x10,0xff,0xf3,0xd2,
    0xcd,0x0c,0x13,0xec,0x5f,0x97,0x44,0x17,0xc4,0xa7,0x7e,0x3d,0x64,0x5d,0x19,0x73,
    0x60,0x81,0x4f,0xdc,0x22,0x2a,0x90,0x88,0x46,0xee,0xb8,0x14,0xde,0x5e,0x0b,0xdb,
    0xe0,0x32,0x3a,0x0a,0x49,0x06,0x24,0x5c,0xc2,0xd3,0xac,0x62,0x91,0x95,0xe4,0x79,
    0xe7,0xc8,0x37,0x6d,0x8d,0xd5,0x4e,0xa9,0x6c,0x56,0xf4,0xea,0x65,0x7a,0xae,0x08,
    0xba,0x78,0x25,0x2e,0x1c,0xa6,0xb4,0xc6,0xe8,0xdd,0x74,0x1f,0x4b,0xbd,0x8b,0x8a,
    0x70,0x3e,0xb5,0x66,0x48,0x03,0xf6,0x0e,0x61,0x35,0x57,0xb9,0x86,0xc1,0x1d,0x9e,
    0xe1,0xf8,0x98,0x11,0x69,0xd9,0x8e,0x94,0x9b,0x1e,0x87,0xe9,0xce,0x55,0x28,0xdf,
    0x8c,0xa1,0x89,0x0d,0xbf,0xe6,0x42,0x68,0x41,0x99,0x2d,0x0f,0xb0,0x54,0xbb,0x16
};

__device__ static void sub_bytes(unsigned char state[16]) {
    for (int i = 0; i < 16; ++i)
        state[i] = d_sbox[state[i]];
}

__device__ static void shift_rows(unsigned char state[16]) {
    unsigned char tmp;
    // Row 1: shift left by 1
    tmp = state[1]; state[1] = state[5]; state[5] = state[9];
    state[9] = state[13]; state[13] = tmp;
    // Row 2: shift left by 2
    tmp = state[2]; state[2] = state[10]; state[10] = tmp;
    tmp = state[6]; state[6] = state[14]; state[14] = tmp;
    // Row 3: shift left by 3 (= right by 1)
    tmp = state[15]; state[15] = state[11]; state[11] = state[7];
    state[7] = state[3]; state[3] = tmp;
}

__device__ static unsigned char xtime(unsigned char a) {
    return (unsigned char)((a << 1) ^ (((a >> 7) & 1) * 0x1b));
}

__device__ static void mix_columns(unsigned char state[16]) {
    for (int c = 0; c < 4; ++c) {
        int i = c * 4;
        unsigned char a0 = state[i], a1 = state[i+1], a2 = state[i+2], a3 = state[i+3];
        unsigned char t = a0 ^ a1 ^ a2 ^ a3;
        state[i]   = a0 ^ xtime(a0 ^ a1) ^ t;
        state[i+1] = a1 ^ xtime(a1 ^ a2) ^ t;
        state[i+2] = a2 ^ xtime(a2 ^ a3) ^ t;
        state[i+3] = a3 ^ xtime(a3 ^ a0) ^ t;
    }
}

__device__ static void add_round_key(unsigned char state[16],
                                      const unsigned char* round_key) {
    for (int i = 0; i < 16; ++i)
        state[i] ^= round_key[i];
}

extern "C" __global__ void aes256_encrypt_kernel(unsigned char* __restrict__ data,
                                       const unsigned char* __restrict__ round_keys,
                                       int num_blocks) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= num_blocks) return;

    unsigned char state[16];
    // Load block
    for (int i = 0; i < 16; ++i)
        state[i] = data[idx * 16 + i];

    // Initial round key addition
    add_round_key(state, round_keys);

    // Rounds 1-13
    for (int r = 1; r <= 13; ++r) {
        sub_bytes(state);
        shift_rows(state);
        mix_columns(state);
        add_round_key(state, round_keys + r * 16);
    }

    // Final round (no MixColumns)
    sub_bytes(state);
    shift_rows(state);
    add_round_key(state, round_keys + 14 * 16);

    // Store result
    for (int i = 0; i < 16; ++i)
        data[idx * 16 + i] = state[i];
}
