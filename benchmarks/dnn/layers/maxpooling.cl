/*#define CL_KERNEL_LOOP(i, n)                        \
  for (int i = get_group_id(0) * get_local_size(0) + get_local_id(0); \
      i < (n);                                       \
      i += get_local_size(0) * get_num_groups(0))
*/

__kernel void MaxPoolForward(int nthreads, __global float* bottom_data, int num,
                             int channels, int height, int width,
                             int pooled_height, int pooled_width, int kernel_h,
                             int kernel_w, int stride_h, int stride_w,
                             int pad_h, int pad_w, __global float* top_data,
                             __global int* mask_data) {
  size_t index = get_global_id(0);
  int pw = index % pooled_width;
  int ph = (index / pooled_width) % pooled_height;
  int c = (index / pooled_width / pooled_height) % channels;
  int n = index / pooled_width / pooled_height / channels;
  int hstart = ph * stride_h - pad_h;
  int wstart = pw * stride_w - pad_w;
  int hend = min(hstart + kernel_h, height);
  int wend = min(wstart + kernel_w, width);
  hstart = max(hstart, 0);
  wstart = max(wstart, 0);

  float maxval = -1;
  int maxidx = -1;
  // bottom_data = bottom_data + (n * channels + c) * height * width;
  for (int h = hstart; h < hend; h++) {
    for (int w = wstart; w < wend; w++) {
      if (bottom_data[h * width + w] > maxval) {
        maxidx = h * width + w;
        maxval = bottom_data[maxidx];
      }
    }
  }
  top_data[index] = maxval;
  mask_data[index] = maxidx;
}

// __kernel void MaxPoolBackward(const int nthreads, __global float* top_diff,
//                               __global int* top_mask, const int num,
//                               const int channels, const int height,
//                               const int width, const int pooled_height,
//                               const int pooled_width, const int kernel_h,
//                               const int kernel_w, const int stride_h,
//                               const int stride_w, const int pad_h,
//                               const int pad_w, __global float* bottom_diff) {
//   int gid = get_global_id(0);
//   int tmp = get_global_size(0);
//   for (int index = gid; index < nthreads; index += tmp) {
//     int w = index % width;
//     int h = (index / width) % height;
//     int c = (index / width / height) % channels;
//     int n = index / width / height / channels;
//     int phstart =
//         (h + pad_h < kernel_h) ? 0 : (h + pad_h - kernel_h) / stride_h + 1;
//     int phend = min((h + pad_h) / stride_h + 1, pooled_height);
//     int pwstart =
//         (w + pad_w < kernel_w) ? 0 : (w + pad_w - kernel_w) / stride_w + 1;
//     int pwend = min((w + pad_w) / stride_w + 1, pooled_width);
//     float gradient = 0;
//     int offset = (n * channels + c) * pooled_height * pooled_width;
//     top_diff += offset;
//     top_mask += offset;
//     for (int ph = phstart; ph < phend; ++ph) {
//       for (int pw = pwstart; pw < pwend; ++pw) {
//         if (top_mask[ph * pooled_width + pw] - 1 == h * width + w) {
//           gradient += top_diff[ph * pooled_width + pw];
//         }
//       }
//     }
//     bottom_diff[index] = gradient;
//   }
// }