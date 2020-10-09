/*
// from im2col.h:

// CL: grid stride looping
#define CL_KERNEL_LOOP(i, n)                        \
  for (int i = get_group_id(0) * get_local_size(0) + get_local_id(0); \
      i < (n);                                       \
      i += get_local_size(0) * get_num_groups(0))

// Kernel for fast unfold+copy
// (borrowed from Caffe: https://github.com/BVLC/caffe/blob/master/src/caffe/layers/conv_layer.cu)
kernel void im2col_kernel(const int n, const global float* im_data, int im_offset,
    const int height, const int width, const int ksize_h, const int ksize_w, const int pad_h,
    const int pad_w, const int stride_h, const int stride_w, const int height_col, const int width_col,
    global float* col_data, int col_offset) {
  global const float *data_im = im_data + im_offset;
  global float *data_col = col_data + col_offset;

  CL_KERNEL_LOOP(index, n) {
    int w_out = index % width_col;
    index /= width_col;
    int h_out = index % height_col;
    int channel_in = index / height_col;
    int channel_out = channel_in * ksize_h * ksize_w;
    int h_in = h_out * stride_h - pad_h;
    int w_in = w_out * stride_w - pad_w;
    data_col += (channel_out * height_col + h_out) * width_col + w_out;
    data_im += (channel_in * height + h_in) * width + w_in;
    for (int i = 0; i < ksize_h; ++i) {
      for (int j = 0; j < ksize_w; ++j) {
        int h = h_in + i;
        int w = w_in + j;
        *data_col = (h >= 0 && w >= 0 && h < height && w < width) ?
          data_im[i * width + j] : 0;
        data_col += height_col * width_col;
      }
    }
  }
}
*/


__kernel void im2colKernel(__global  float  * input,
											__global  float  * output,
											const     uint2  inputDimensions,
											const     uint2  maskDimensions,
                      const     uint2  strDimensions,
                      const     uint2  padVertDimensions,
                      const     uint2  padHoriDImensions,
                      const     uint   channel,
                      const     uint   batch)
{
    uint tid   = get_global_id(0);
    
    uint width  = inputDimensions.x;
    uint height = inputDimensions.y;
    
    uint maskWidth  = maskDimensions.x;
    uint maskHeight = maskDimensions.y;

    uint fieldWidth = (width - maskWidth + padHoriDImensions.x + padHoriDImensions.y) / strDimensions.x + 1;
	  uint fieldHeight = (height - maskHeight + padVertDimensions.x + padVertDimensions.y) / strDimensions.y + 1;

    uint realWidth = fieldHeight * fieldWidth;
    uint realHeight = maskHeight * maskWidth;

    uint batch_id = tid / (channel * realWidth);
    uint pid = tid % (channel * realWidth);
    uint channel_id = pid / realWidth;
    pid = pid % realWidth;

    uint cut_id = tid / realWidth; // nth matrix, ignoring channel and batch, note 0th

    if(pid >= realWidth || channel_id >= channel || batch_id >= batch)
		  return;

    uint x = pid%fieldWidth;
    uint y = pid/fieldWidth;
    for (int i = 0; i < maskHeight; i++) {
			for (int j = 0; j < maskWidth; j++) {
					
					uint indexOut =  (y + x * fieldHeight) + (i + j * maskHeight)* fieldHeight * fieldWidth + cut_id * fieldHeight * fieldWidth * maskHeight * maskWidth; 
          // the output is row wise, meaning for a new channel, a new matrix is added to the down side of the originial one
          uint indexRawVert = (i + y * strDimensions.y) - padVertDimensions.x;
          uint indexRawHori = (j + x *strDimensions.x) - padHoriDImensions.x;
					//uint indexIn = (i + y * strDimensions.y) * width + (j + x*strDimensions.x);

          if (indexRawVert >= 0 && indexRawVert < height && indexRawHori  >=0 && indexRawHori  < width){
          // the input is considered to be row major
						uint indexIn = indexRawVert * width + indexRawHori + cut_id * width * height;
						output[indexOut] = input[indexIn];
					}else{
						output[indexOut] = 0.0;
					}

			}
		}
}

