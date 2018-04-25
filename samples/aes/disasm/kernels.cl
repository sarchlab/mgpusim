void SubBytes(uchar* input, __global uchar *s) {
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

void MixColumns(uchar* arr) {
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

void ShiftRows(uchar* input) {
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

void AddRoundKey(uchar* state, __global uint* expanded_key, int offset) {
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

__kernel void Encrypt(__global uchar* input, 
                      __global uint* expanded_key, 
                      __global uchar* s) {
  uchar state[16];

  int tid = get_global_id(0);

  for (int i = 0; i < 16; i++) {
    state[i] = input[tid * 16 + i];
  }

  AddRoundKey(state, expanded_key, 0);

  /*for (int i = 1; i < 14; i++) {*/
    /*SubBytes(state, s);*/
    /*ShiftRows(state);*/
    /*MixColumns(state);*/
    /*AddRoundKey(state, expanded_key, i * 4);*/
  /*}*/

  /*SubBytes(state, s);*/
  /*ShiftRows(state);*/
  /*AddRoundKey(state, expanded_key, 14 * 4);*/

  for (int i = 0; i < 16; i++) {
    input[tid * 16 + i] = state[i];
  }
}
