
__kernel void im2col(__global float *input, __global float *output,
                     const uint2 inputDimensions, const uint2 maskDimensions,
                     const uint2 stride, const uint2 pad, const uint2 dilation,
                     const uint channel, const uint batch) {
  uint tid = get_global_id(0);

  int width = inputDimensions.x;
  int height = inputDimensions.y;

  int maskWidth = maskDimensions.x;
  int maskHeight = maskDimensions.y;

  int effKernelHeight = (maskDimensions.y - 1) * dilation.y + 1;
  int effKernelWidth = (maskDimensions.x - 1) * dilation.x + 1;

  int fieldHeight = (height - effKernelHeight + 2 * pad.y) / stride.y + 1;
  int fieldWidth = (width - effKernelWidth + 2 * pad.x) / stride.x + 1;

  int outWidth = fieldHeight * fieldWidth * batch;
  int outHeight = maskHeight * maskWidth * channel;

  int frame_size = width * height;
  int picture_size = channel * frame_size;
  int mask_size = maskWidth * maskHeight;

  int batch_id = tid / (fieldWidth * fieldHeight);
  int block_id = tid % (fieldWidth * fieldHeight);
  int block_x = block_id % fieldWidth;
  int block_y = block_id / fieldWidth;

  if (batch_id >= batch)
    return;

  for (int i = 0; i < outHeight; i++) {
    int channel_id = i / mask_size;
    int y = i % mask_size / maskWidth;
    int x = i % maskWidth;

    int real_x = block_x * stride.x - pad.x + dilation.x * x;
    int real_y = block_y * stride.y - pad.y + dilation.y * y;
    int input_index = batch_id * picture_size + channel_id * frame_size +
                      real_y * width + real_x;
    int output_index = i * outWidth + tid;

    float out = 0;
    if (real_x >= 0 && real_y >= 0 && real_x < width && real_y < height) {
      out = input[input_index];
    }
    output[output_index] = out;
  }
}
