__kernel void col2imKernel(
    const int input_h, const int input_w, const int channels,
    const int output_h, const int output_w,
    const int kernel_h, const int kernel_w,
    const int pad_h, const int pad_w,
    const int stride_h, const int stride_w,
    const int dilation_h, const int dilation_w,
    const global float *col_buffer, const int col_offset,
    __global float *im_buffer, const int im_offset) {
        
  const int x_x = get_global_id(0) + pad_w;
  const int x_y = ((int) get_global_id(1)) % input_h + pad_h;
  const int channel = ((int) get_global_id(1)) / input_h;
  const int kernel_extent_w = (kernel_w - 1) * dilation_w + 1;
  const int kernel_extent_h = (kernel_h - 1) * dilation_h + 1;
  const int col_channel_shift = channel * kernel_w * kernel_h * output_h * output_w + col_offset;
  const int x_channel_shift = channel * input_h * input_w + im_offset;
  const int t_y_begin = (x_y < kernel_extent_h) ? 0 : (x_y - kernel_extent_h) / stride_h + 1;
  const int t_y_end = min(x_y / stride_h + 1, output_h);
  const int t_x_begin = (x_x < kernel_extent_w) ? 0 : (x_x - kernel_extent_w) / stride_w + 1;
  const int t_x_end = min(x_x / stride_w + 1, output_w);
  if (x_x < input_w + pad_w && channel < channels) {
    float val = 0;
    for (int t_y = t_y_begin; t_y < t_y_end; ++t_y) {
      for (int t_x = t_x_begin; t_x < t_x_end; ++t_x) {
        int w_y = x_y - t_y * stride_h;
        int w_x = x_x - t_x * stride_w;
        if (w_y % dilation_h == 0 && w_x % dilation_w == 0) {
          w_y /= dilation_h;
          w_x /= dilation_w;
          val += col_buffer[col_channel_shift
                              + (w_x + w_y * kernel_w) * output_h * output_w
                              + t_y * output_w
                              + t_x];
        }
      }
    }
    im_buffer[x_channel_shift + (x_y - pad_h) * input_w + x_x - pad_w] = val;
  }
}