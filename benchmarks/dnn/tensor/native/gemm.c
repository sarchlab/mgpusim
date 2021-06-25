#include <CL/opencl.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>

char *read_file(const char *filename) {
  char *buffer = 0;
  long length;
  FILE *f = fopen(filename, "rb");

  if (f) {
    fseek(f, 0, SEEK_END);
    length = ftell(f);
    fseek(f, 0, SEEK_SET);
    buffer = (char *)(malloc(length));
    if (buffer) {
      fread(buffer, 1, length, f);
    }
    fclose(f);
  }

  return buffer;
}

// m = 17, n = 11, k = 9
// alpha * a * b + beta * c
// alpha = 3.1, beta = 4.2
// d = numpy.matmul(a, b) * alpha + beta * c

int main(int argc, char *argv[]) {
    int m = 17;
    int n = 11;
    int k = 9;
    float alpha = 3.1;
    float beta = 4.2;

  float h_input_a[] = {
      0.19042209, 0.38606963, 0.56103939, 0.450782, 0.63485176,
     0.71801504, 0.8682696 , 0.51338282, 0.47060541, 0.90531313,
     0.38953205, 0.52672663, 0.41814208, 0.49757131, 0.78637201,
     0.6384301 , 0.22371721, 0.49305215, 0.85131365, 0.42075421,
     0.05449744, 0.92031014, 0.81778452, 0.61031005, 0.74141907,
     0.21780084, 0.29527322, 0.42671307, 0.7024013 , 0.98173889,
     0.21757945, 0.03267397, 0.95583688, 0.41935899, 0.23400085,
     0.99137392, 0.62449197, 0.52802359, 0.77684426, 0.93525841,
     0.75636883, 0.9505029 , 0.37665829, 0.03272443, 0.18701128,
     0.98376324, 0.37296266, 0.85275978, 0.32997702, 0.21481603,
     0.84988021, 0.01338554, 0.54842581, 0.29181977, 0.71963503,
     0.99375905, 0.92533084, 0.62930432, 0.94888833, 0.24722147,
     0.91476533, 0.83032015, 0.7387861 , 0.21793444, 0.13129791,
     0.3204544 , 0.4369123 , 0.33947677, 0.73218619, 0.50465326,
     0.93626095, 0.24626508, 0.88754563, 0.40341395, 0.90518423,
     0.606578  , 0.75551207, 0.94269105, 0.73798436, 0.33857609,
     0.09411834, 0.63829888, 0.16217327, 0.35634068, 0.42909213,
     0.57262646, 0.29109585, 0.66390382, 0.36248932, 0.15684851,
     0.48463416, 0.71711204, 0.81084781, 0.81065799, 0.52702118,
     0.28193737, 0.60122373, 0.65482472, 0.9476652 , 0.24775301,
     0.90052255, 0.66377207, 0.29122121, 0.198443  , 0.78704806,
     0.88412466, 0.70187439, 0.65289548, 0.65000689, 0.53470062,
     0.52916923, 0.22704818, 0.84009412, 0.57393336, 0.05932007,
     0.42836673, 0.23218359, 0.15174416, 0.29559115, 0.17953001,
     0.63490091, 0.68141472, 0.6154076 , 0.78553265, 0.75543813,
     0.67123114, 0.18651229, 0.44928971, 0.33024056, 0.67492924,
     0.17725189, 0.61122706, 0.11670089, 0.1784668 , 0.80756339,
     0.79362398, 0.1155913 , 0.98472861, 0.48826153, 0.37292461,
     0.17142945, 0.50859467, 0.01068719, 0.21239646, 0.2136699 ,
     0.48886655, 0.50352522, 0.23215374, 0.89803985, 0.39999043,
     0.32859328, 0.15635657, 0.05363765
  };

  float h_input_b[] = {
        0.80717168, 0.19557895, 0.04398682, 0.53098358, 0.51295872,
       0.579826  , 0.62898664, 0.17164213, 0.87204565, 0.42216476,
       0.533152  , 0.31985664, 0.64720486, 0.56298073, 0.44982443,
       0.85116818, 0.97758078, 0.93708569, 0.72110124, 0.12956963,
       0.22914527, 0.22542768, 0.72250548, 0.26402763, 0.98522945,
       0.86233052, 0.20792001, 0.70415942, 0.40508114, 0.83277519,
       0.38090795, 0.51016008, 0.81539314, 0.42034111, 0.69412815,
       0.91048267, 0.66221871, 0.46600039, 0.98334095, 0.50965538,
       0.12257083, 0.67963624, 0.88125952, 0.51949196, 0.77615909,
       0.19312383, 0.86202196, 0.77277418, 0.63206971, 0.02336199,
       0.36180138, 0.13784341, 0.75271599, 0.7225816 , 0.56283408,
       0.49703285, 0.70213261, 0.5248285 , 0.48809128, 0.6525293 ,
       0.76720359, 0.78559459, 0.31936372, 0.03127426, 0.22955171,
       0.33159755, 0.45869835, 0.80865137, 0.79226911, 0.31205392,
       0.22641916, 0.93774274, 0.28942951, 0.32575207, 0.60232379,
       0.14638335, 0.71423965, 0.95698797, 0.79889702, 0.0614126 ,
       0.18582932, 0.84493905, 0.74990925, 0.67473464, 0.5478188 ,
       0.12033698, 0.73823212, 0.42842718, 0.79085304, 0.81684295,
       0.74353177, 0.69256141, 0.66408693, 0.98016319, 0.3916768 ,
       0.00432683, 0.91120368, 0.08543805, 0.63215705
   };

   float h_input_c[] = {
           0.06021883, 0.15760754, 0.01433924, 0.00907472, 0.6582698 ,
          0.59459894, 0.6891485 , 0.15469464, 0.63171817, 0.2851064 ,
          0.02541673, 0.28786125, 0.22023797, 0.2874199 , 0.30363558,
          0.1354755 , 0.95236479, 0.60246108, 0.42527619, 0.5207891 ,
          0.32886278, 0.92155838, 0.81440863, 0.85123608, 0.95085352,
          0.9679652 , 0.40322454, 0.63084227, 0.70599522, 0.40761364,
          0.11893196, 0.67409882, 0.05876277, 0.44908324, 0.63916472,
          0.07658133, 0.36088129, 0.98382992, 0.96409638, 0.86163647,
          0.02332475, 0.39311148, 0.76829579, 0.28419138, 0.22866472,
          0.43749643, 0.74197534, 0.8912028 , 0.82411566, 0.81463139,
          0.33496463, 0.60028196, 0.32856723, 0.46512764, 0.82675432,
          0.97107976, 0.50703879, 0.47668873, 0.81119519, 0.73353598,
          0.8093412 , 0.8706791 , 0.73471486, 0.70246836, 0.93934264,
          0.46320568, 0.81235564, 0.9615337 , 0.24847096, 0.07627165,
          0.52279121, 0.26313899, 0.33004516, 0.83654756, 0.0939848 ,
          0.95967984, 0.31643281, 0.61382422, 0.95396035, 0.99466388,
          0.90831305, 0.13320899, 0.37774924, 0.30486501, 0.15388745,
          0.16168485, 0.2066227 , 0.63754892, 0.85221128, 0.79191982,
          0.54601889, 0.27124935, 0.48499467, 0.70781456, 0.40936838,
          0.59998377, 0.00108625, 0.94725314, 0.40386108, 0.65714232,
          0.8148504 , 0.65001935, 0.45969956, 0.96448363, 0.39925587,
          0.07320006, 0.78233348, 0.09450617, 0.67563919, 0.05232766,
          0.03370994, 0.5431951 , 0.21489358, 0.21209878, 0.86239488,
          0.68214264, 0.98307162, 0.74800095, 0.75549118, 0.8160952 ,
          0.65433655, 0.35946335, 0.39233251, 0.93736204, 0.24588189,
          0.9039361 , 0.99388234, 0.2149501 , 0.71395424, 0.72825518,
          0.27986103, 0.6084787 , 0.54518502, 0.05092402, 0.81795736,
          0.31525055, 0.59697824, 0.93238942, 0.61278883, 0.82314124,
          0.81815899, 0.94293836, 0.66639106, 0.89507535, 0.40466901,
          0.36284579, 0.45994118, 0.11535543, 0.3169623 , 0.04802341,
          0.02227552, 0.66563584, 0.97255279, 0.25603985, 0.50932593,
          0.99791928, 0.77930128, 0.82889288, 0.24386979, 0.6211909 ,
          0.88452383, 0.08176916, 0.91370645, 0.38033733, 0.28980826,
          0.76778657, 0.7077874 , 0.92357212, 0.71213796, 0.94550783,
          0.90299273, 0.62340107, 0.10913174, 0.63649326, 0.54013901,
          0.81982848, 0.71005775, 0.80437056, 0.73410064, 0.68429516,
          0.08632411, 0.85049441, 0.81694899, 0.94886611, 0.34765122,
          0.67061503, 0.25433614
    };

    float h_out_d[] = {
    9.50147226,  9.56379998,  9.92547853,  8.06840903, 10.71838503,
       13.50953468, 10.68890077,  5.97082981,  9.62835896,  7.46623165,
        8.20768418, 10.77432526,  8.99880276, 10.15548058,  9.76195253,
        8.5947643 , 15.01253619, 10.86180075,  6.679709  , 10.19964205,
        7.42449513, 11.99727481, 12.62153148, 11.92414037, 13.32930454,
       12.44761082, 10.05779681, 13.44879936, 11.25270259,  5.67211523,
        9.24033725, 10.02384515,  8.13145476, 11.40512313, 11.86537997,
       10.22024671, 10.46206236, 12.55680908, 16.80214633, 12.50181393,
        6.30674072,  8.54401092,  8.17302213,  9.4433235 , 10.38081893,
        9.94433816, 14.06541504, 13.53881973, 11.83922517, 14.74554261,
       10.4081802 ,  8.09737022,  9.26890498,  9.59551126, 11.88400858,
       13.43774535,  8.99150819,  9.08970264, 11.2593964 , 10.84238432,
       13.40254195, 12.07484182,  8.65183594,  9.09933749, 10.17215713,
        9.09731034, 17.32981796, 15.90653068, 14.52396013, 12.25274104,
       14.16495119, 16.83951694, 12.95333374, 11.73980658, 11.50061733,
       13.67339824, 13.05101545, 10.58452911, 11.60894748, 10.73210603,
        9.58767206,  7.55001529, 10.56537279,  8.05353072,  5.02195272,
        5.47019876,  6.66503887,  8.83170321, 14.57285387, 12.15069107,
       13.24161654, 11.17501183, 10.95834807, 15.13082619, 11.34463743,
        9.03890773,  8.53722696, 12.01306023, 11.26530663, 10.10550724,
        9.2853414 ,  9.46452494,  8.04104414,  9.79251657,  9.38457196,
        6.04405063,  6.89194254,  6.69474275,  8.18623329,  6.53694465,
       11.76112239, 12.87981941, 12.63740537, 11.20282454, 13.66969427,
       17.00872497, 13.79354746,  9.64005185, 12.73752899, 11.47180229,
       12.67280653, 11.51901634, 12.2144568 , 14.01235136,  9.39609912,
       13.199169  , 17.75722891, 10.48890968,  9.92524773,  9.78378053,
        7.16964917, 11.06573089, 10.7575678 ,  6.15215268, 10.65541717,
        8.70387876, 10.06959651, 11.85166408,  9.98875397,  8.16838992,
        9.45379552, 10.00696025,  9.16108759, 13.25304692, 11.18503445,
       10.84328905,  9.57675568,  9.03062231, 12.4193434 ,  7.94774311,
        4.52540741, 10.16856215, 10.73908173,  8.88852665,  8.71401315,
       11.14435572, 10.64632765,  9.98218212,  7.48980969, 11.75858669,
       10.01078124,  3.66865628,  9.37267215,  6.03404904,  6.77667908,
       10.60400553,  7.97807298, 11.59082925, 10.20017458,  8.77790368,
       11.79075318,  8.08185072,  4.73039966,  9.49703342,  7.54939716,
       10.4945828 ,  9.27160078,  8.15545052, 10.16968401,  9.00973549,
        5.97154347,  9.74251229,  8.93538183,  7.9576831 ,  6.27548056,
        7.69141932,  6.42801278
    };

  // Device input buffers
  cl_mem d_input_a;
  cl_mem d_input_b;
  cl_mem d_input_c;
  cl_mem d_output;


  cl_platform_id cpPlatform;  // OpenCL platform
  cl_device_id device_id;     // device ID
  cl_context context;         // context
  cl_command_queue queue;     // command queue
  cl_program program;         // program
  cl_kernel kernel;           // kernel

  size_t globalSize, localSize;
  cl_int err;

  // Number of work items in each local work group
  localSize = 64;

  // Number of total work items
  globalSize = m*n;

  // Bind to platform
  err = clGetPlatformIDs(1, &cpPlatform, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to get platform IDs");
    exit(1);
  }

  // Get ID for the device
  err = clGetDeviceIDs(cpPlatform, CL_DEVICE_TYPE_GPU, 1, &device_id, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to get device IDs");
    exit(1);
  }

  // Create a context
  context = clCreateContext(0, 1, &device_id, NULL, NULL, &err);
  if (err != CL_SUCCESS) {
    printf("fail to get create context");
    exit(1);
  }

  // Create a command queue
  queue = clCreateCommandQueue(context, device_id, 0, &err);
  if (err != CL_SUCCESS) {
    printf("fail to get create queue");
    exit(1);
  }

  // Create the compute program from the source buffer
  char *kernelSource = read_file("gemm.cl");
  program = clCreateProgramWithSource(context, 1, (const char **)&kernelSource,
                                      NULL, &err);
  if (err != CL_SUCCESS) {
    printf("fail to create program with source");
    exit(1);
  }

  // Build the program executable
  err = clBuildProgram(program, 0, NULL, NULL, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to build program");
    if (err == CL_BUILD_PROGRAM_FAILURE) {
      // Determine the size of the log
      size_t log_size;
      clGetProgramBuildInfo(program, device_id, CL_PROGRAM_BUILD_LOG, 0, NULL,
                            &log_size);

      // Allocate memory for the log
      char *log = (char *)malloc(log_size);

      // Get the log
      clGetProgramBuildInfo(program, device_id, CL_PROGRAM_BUILD_LOG, log_size,
                            log, NULL);

      // Print the log
      printf("%s\n", log);
    }
    exit(1);
  }

  // Create the compute kernel in the program we wish to run
  kernel = clCreateKernel(program, "gemm", &err);
  if (err != CL_SUCCESS) {
    printf("fail to create kernel %d\n", err);
    exit(1);
  }

  // Create the input and output arrays in device memory for our calculation
  d_input_a =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*m*n , NULL, NULL);
  d_input_b =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*n*k, NULL, NULL);
  d_input_c =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*m*n, NULL, NULL);
  d_output =
      clCreateBuffer(context, CL_MEM_WRITE_ONLY, sizeof(float)*m*n, NULL, NULL);

  float *h_out = malloc(m * n * sizeof(float));

  // Write our data set into the input array in device memory
  err = clEnqueueWriteBuffer(queue, d_input_a, CL_TRUE, 0, m*k,
                             h_input_a, 0, NULL, NULL);
  err = clEnqueueWriteBuffer(queue, d_input_b, CL_TRUE, 0, n*k,
                               h_input_b, 0, NULL, NULL);
  err = clEnqueueWriteBuffer(queue, d_input_c, CL_TRUE, 0, m*n,
                               h_input_c, 0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue write buffer %d", err);
    exit(1);
  }

  // Set the arguments to our compute kernel
  err |= clSetKernelArg(kernel, 0, sizeof(cl_int), &m);
  err |= clSetKernelArg(kernel, 1, sizeof(cl_int), &n);
  err |= clSetKernelArg(kernel, 2, sizeof(cl_int), &k);
  err |= clSetKernelArg(kernel, 3, sizeof(cl_float), &alpha);
  err |= clSetKernelArg(kernel, 4, sizeof(cl_float), &beta);
  err |= clSetKernelArg(kernel, 5, sizeof(cl_mem), &d_input_a);
  err |= clSetKernelArg(kernel, 6, sizeof(cl_mem), &d_input_b);
  err |= clSetKernelArg(kernel, 7, sizeof(cl_mem), &d_input_c);
  err |= clSetKernelArg(kernel, 8, sizeof(cl_mem), &d_output);

  // Execute the kernel over the entire range of the data set
  err = clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &globalSize, &localSize,
                               0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue ND Range Kernel");
    exit(1);
  }

  // Wait for the command queue to get serviced before reading back results
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to finish");
    exit(1);
  }

  // Read the results from the device
  clEnqueueReadBuffer(queue, d_output, CL_TRUE, 0,
                      m * n * sizeof(float), h_out,
                      0, NULL, NULL);
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to read buffer");
    exit(1);
  }

  printf("\n\nOut From GPU:\n");
  for (int r = 0; r < m; r++) {
    printf("%d: ", r);

    for (int c = 0; c < k; c++) {
      printf("%f ", h_out[r * 9 + c]);
    }

    printf("\n");
  }

  printf("\n\nOut:\n");
  for (int r = 0; r < m; r++) {
    printf("%d: ", r);

    for (int c = 0; c < k; c++) {
      printf("%f ", h_out_d[r * 9 + c]);
    }

    printf("\n");
  }

  // release OpenCL resources
  clReleaseProgram(program);
  clReleaseKernel(kernel);
  clReleaseCommandQueue(queue);
  clReleaseContext(context);

  return 0;
}
