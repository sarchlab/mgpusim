

__kernel void flattenKernel(__global  float  * input,
											__global  float  * output,
                      const     uint   Inputchannel,
                      const     uint   Outputchannel,
                      const     uint   Height,
                      const     uint   Width)
{
    uint tid   = get_global_id(0);
    uint oc = tid / (Inputchannel * Height * Width);
    uint rest = tid % (Inputchannel * Height * Width);
    uint ic = rest / (Height * Width);
    rest = rest % (Height * Width);
    uint H = rest / Height;
    uint W = rest % Height;

    if(oc >= Outputchannel || ic >= Inputchannel || H >= Height || W >= Width)
		  return;

    output[oc * Inputchannel * Height * Width + ic * Height * Width + (Height - H -1) * Width + (Width - W - 1)] = input[tid];

}

