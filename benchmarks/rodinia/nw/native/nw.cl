// #define BLOCK_SIZE 8
#define SCORE(i, j, block_size) input_itemsets_l[j + i * (block_size + 1)]
#define REF(i, j, block_size) reference_l[j + i * block_size]

int maximum(int a, int b, int c) {
  int k;
  if (a <= b)
    k = b;
  else
    k = a;

  if (k <= c)
    return (c);
  else
    return (k);
}

__kernel void nw_kernel1(__global int* reference_d,
                         __global int* input_itemsets_d,
                         __global int* output_itemsets_d,
                         __local int* input_itemsets_l,
                         __local int* reference_l, int cols, int penalty,
                         int blk, int block_size, int block_width, int worksize,
                         int offset_r, int offset_c) {
  // Block index
  int bx = get_group_id(0);
  // int bx = get_global_id(0)/BLOCK_SIZE;

  // Thread index
  int tx = get_local_id(0);

  // Base elements
  int base = offset_r * cols + offset_c;

  int b_index_x = bx;
  int b_index_y = blk - 1 - bx;

  int index = base + cols * block_size * b_index_y + block_size * b_index_x +
              tx + (cols + 1);
  int index_n =
      base + cols * block_size * b_index_y + block_size * b_index_x + tx + (1);
  int index_w =
      base + cols * block_size * b_index_y + block_size * b_index_x + (cols);
  int index_nw = base + cols * block_size * b_index_y + block_size * b_index_x;

  if (tx == 0) {
    SCORE(tx, 0, block_size) = input_itemsets_d[index_nw + tx];
  }

  barrier(CLK_LOCAL_MEM_FENCE);

  for (int ty = 0; ty < block_size; ty++)
    REF(ty, tx, block_size) = reference_d[index + cols * ty];

  barrier(CLK_LOCAL_MEM_FENCE);

  SCORE((tx + 1), 0, block_size) = input_itemsets_d[index_w + cols * tx];

  barrier(CLK_LOCAL_MEM_FENCE);

  SCORE(0, (tx + 1), block_size) = input_itemsets_d[index_n];

  barrier(CLK_LOCAL_MEM_FENCE);

  for (int m = 0; m < block_size; m++) {
    if (tx <= m) {
      int t_index_x = tx + 1;
      int t_index_y = m - tx + 1;

      SCORE(t_index_y, t_index_x, block_size) =
          maximum(SCORE((t_index_y - 1), (t_index_x - 1), block_size) +
                      REF((t_index_y - 1), (t_index_x - 1), block_size),
                  SCORE((t_index_y), (t_index_x - 1), block_size) - (penalty),
                  SCORE((t_index_y - 1), (t_index_x), block_size) - (penalty));
    }
    barrier(CLK_LOCAL_MEM_FENCE);
  }

  barrier(CLK_LOCAL_MEM_FENCE);

  for (int m = block_size - 2; m >= 0; m--) {
    if (tx <= m) {
      int t_index_x = tx + block_size - m;
      int t_index_y = block_size - tx;

      SCORE(t_index_y, t_index_x, block_size) =
          maximum(SCORE((t_index_y - 1), (t_index_x - 1), block_size) +
                      REF((t_index_y - 1), (t_index_x - 1), block_size),
                  SCORE((t_index_y), (t_index_x - 1), block_size) - (penalty),
                  SCORE((t_index_y - 1), (t_index_x), block_size) - (penalty));
    }

    barrier(CLK_LOCAL_MEM_FENCE);
  }

  for (int ty = 0; ty < block_size; ty++)
    input_itemsets_d[index + cols * ty] = SCORE((ty + 1), (tx + 1), block_size);

  return;
}

__kernel void nw_kernel2(__global int* reference_d,
                         __global int* input_itemsets_d,
                         __global int* output_itemsets_d,
                         __local int* input_itemsets_l,
                         __local int* reference_l, int cols, int penalty,
                         int blk, int block_size, int block_width, int worksize,
                         int offset_r, int offset_c) {
  int bx = get_group_id(0);
  // int bx = get_global_id(0)/BLOCK_SIZE;

  // Thread index
  int tx = get_local_id(0);

  // Base elements
  int base = offset_r * cols + offset_c;

  int b_index_x = bx + block_width - blk;
  int b_index_y = block_width - bx - 1;

  int index = base + cols * block_size * b_index_y + block_size * b_index_x +
              tx + (cols + 1);
  int index_n =
      base + cols * block_size * b_index_y + block_size * b_index_x + tx + (1);
  int index_w =
      base + cols * block_size * b_index_y + block_size * b_index_x + (cols);
  int index_nw = base + cols * block_size * b_index_y + block_size * b_index_x;

  if (tx == 0) SCORE(tx, 0, block_size) = input_itemsets_d[index_nw];

  for (int ty = 0; ty < block_size; ty++)
    REF(ty, tx, block_size) = reference_d[index + cols * ty];

  barrier(CLK_LOCAL_MEM_FENCE);

  SCORE((tx + 1), 0, block_size) = input_itemsets_d[index_w + cols * tx];

  barrier(CLK_LOCAL_MEM_FENCE);

  SCORE(0, (tx + 1), block_size) = input_itemsets_d[index_n];

  barrier(CLK_LOCAL_MEM_FENCE);

  for (int m = 0; m < block_size; m++) {
    if (tx <= m) {
      int t_index_x = tx + 1;
      int t_index_y = m - tx + 1;

      SCORE(t_index_y, t_index_x, block_size) =
          maximum(SCORE((t_index_y - 1), (t_index_x - 1), block_size) +
                      REF((t_index_y - 1), (t_index_x - 1), block_size),
                  SCORE((t_index_y), (t_index_x - 1), block_size) - (penalty),
                  SCORE((t_index_y - 1), (t_index_x), block_size) - (penalty));
    }
    barrier(CLK_LOCAL_MEM_FENCE);
  }

  for (int m = block_size - 2; m >= 0; m--) {
    if (tx <= m) {
      int t_index_x = tx + block_size - m;
      int t_index_y = block_size - tx;

      SCORE(t_index_y, t_index_x, block_size) =
          maximum(SCORE((t_index_y - 1), (t_index_x - 1), block_size) +
                      REF((t_index_y - 1), (t_index_x - 1), block_size),
                  SCORE((t_index_y), (t_index_x - 1), block_size) - (penalty),
                  SCORE((t_index_y - 1), (t_index_x), block_size) - (penalty));
    }

    barrier(CLK_LOCAL_MEM_FENCE);
  }

  for (int ty = 0; ty < block_size; ty++)
    input_itemsets_d[index + ty * cols] = SCORE((ty + 1), (tx + 1), block_size);

  return;
}
