__kernel void im2colKernelNCHW(
    __global float* input, __global float* output, const uint2 inputDimensions,
    const uint2 maskDimensions, const uint2 strDimensions,
    const uint2 padVertDimensions, const uint2 padHoriDImensions,
    const uint2 dilation, const uint channel, const uint batch) {
  uint tid = get_global_id(0);

  uint width = inputDimensions.x;
  uint height = inputDimensions.y;

  uint maskWidth = maskDimensions.x;
  uint maskHeight = maskDimensions.y;

  uint fieldWidth = (width - maskWidth - (maskWidth - 1) * dilation.x +
                     padHoriDImensions.x + padHoriDImensions.y) /
                        strDimensions.x +
                    1;
  uint fieldHeight = (height - maskHeight - (maskHeight - 1) * dilation.y +
                      padVertDimensions.x + padVertDimensions.y) /
                         strDimensions.y +
                     1;

  uint outWidth = fieldHeight * fieldWidth * batch;
  uint outHeight = maskHeight * maskWidth * channel;

  uint frame_size = width * height;
  uint picture_size = channel * frame_size;

  uint batch_id = tid / (fieldWidth * fieldHeight);
  uint block_id = tid % (fieldWidth * fieldHeight);
  uint block_x = block_id % fieldWidth;
  uint block_y = block_id / fieldWidth;

  if (batch_id >= batch) return;

  uint channel_id = 0;
  uint y = 0;
  uint x = 0;
  for (int i = 0; i < outHeight; i++) {
    int real_x =
        x + block_x * strDimensions.x - padHoriDImensions.x + dilation.x * x;
    int real_y =
        y + block_y * strDimensions.y - padVertDimensions.x + dilation.y * y;
    int input_index = batch_id * picture_size + channel_id * frame_size +
                      real_y * width + real_x;
    int output_index = i * outWidth + tid;

    float out = 0;
    if (real_x >= 0 && real_y >= 0 && real_x < width && real_y < height) {
      out = input[input_index];
    }
    output[output_index] = out;

    x++;
    if (x >= maskWidth) {
      x = 0;
      y++;
      if (y >= maskHeight) {
        y = 0;
        channel_id++;
      }
    }
  }
}

__kernel void im2colKernelCNHW(
    __global float* input, __global float* output, const uint2 inputDimensions,
    const uint2 maskDimensions, const uint2 strDimensions,
    const uint2 padVertDimensions, const uint2 padHoriDImensions,
    const uint2 dilation, const uint channel, const uint batch) {
  uint tid = get_global_id(0);

  uint width = inputDimensions.x;
  uint height = inputDimensions.y;

  uint maskWidth = maskDimensions.x;
  uint maskHeight = maskDimensions.y;

  uint fieldWidth = (width - maskWidth - (maskWidth - 1) * dilation.x +
                     padHoriDImensions.x + padHoriDImensions.y) /
                        strDimensions.x +
                    1;
  uint fieldHeight = (height - maskHeight - (maskHeight - 1) * dilation.y +
                      padVertDimensions.x + padVertDimensions.y) /
                         strDimensions.y +
                     1;

  uint outWidth = fieldHeight * fieldWidth * batch;
  uint outHeight = maskHeight * maskWidth * channel;

  uint frame_size = width * height;
  uint picture_size = channel * frame_size;

  uint batch_id = tid / (fieldWidth * fieldHeight);
  uint block_id = tid % (fieldWidth * fieldHeight);
  uint block_x = block_id % fieldWidth;
  uint block_y = block_id / fieldWidth;

  if (batch_id >= batch) return;

  uint channel_id = 0;
  uint y = 0;
  uint x = 0;
  for (int i = 0; i < outHeight; i++) {
    int real_x =
        x + block_x * strDimensions.x - padHoriDImensions.x + dilation.x * x;
    int real_y =
        y + block_y * strDimensions.y - padVertDimensions.x + dilation.y * y;
    int input_index = batch_id * frame_size + channel_id * frame_size * batch +
                      real_y * width + real_x;
    int output_index = i * outWidth + tid;

    float out = 0;
    if (real_x >= 0 && real_y >= 0 && real_x < width && real_y < height) {
      out = input[input_index];
    }
    // out = channel_id * frame_size * channel + batch_id * frame_size;
    output[output_index] = out;

    x++;
    if (x >= maskWidth) {
      x = 0;
      y++;
      if (y >= maskHeight) {
        y = 0;
        channel_id++;
      }
    }
  }
}
