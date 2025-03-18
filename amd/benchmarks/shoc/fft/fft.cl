// -*- c++ -*-

// This code uses algorithm described in:
// "Fitting FFT onto G80 Architecture". Vasily Volkov and Brian Kazian, UC Berkeley CS258 project report. May 2008.

#ifdef SINGLE_PRECISION
#define T float
#define T2 float2
#elif K_DOUBLE_PRECISION
#pragma OPENCL EXTENSION cl_khr_fp64: enable
#define T double
#define T2 double2
#elif AMD_DOUBLE_PRECISION
#pragma OPENCL EXTENSION cl_amd_fp64: enable
#define T double
#define T2 double2
#endif

#ifndef M_PI
# define M_PI 3.14159265358979323846f
#endif

#ifndef M_SQRT1_2
# define M_SQRT1_2      0.70710678118654752440f
#endif


#define exp_1_8   (T2)(  1, -1 )//requires post-multiply by 1/sqrt(2)
#define exp_1_4   (T2)(  0, -1 )
#define exp_3_8   (T2)( -1, -1 )//requires post-multiply by 1/sqrt(2)

#define iexp_1_8   (T2)(  1, 1 )//requires post-multiply by 1/sqrt(2)
#define iexp_1_4   (T2)(  0, 1 )
#define iexp_3_8   (T2)( -1, 1 )//requires post-multiply by 1/sqrt(2)


inline void globalLoads8(T2 *data, __global T2 *in, int stride){
    for( int i = 0; i < 8; i++ )
        data[i] = in[i*stride];
}


inline void globalStores8(T2 *data, __global T2 *out, int stride){
    int reversed[] = {0,4,2,6,1,5,3,7};

//#pragma unroll
    for( int i = 0; i < 8; i++ )
        out[i*stride] = data[reversed[i]];
}


inline void storex8( T2 *a, __local T *x, int sx ) {
    int reversed[] = {0,4,2,6,1,5,3,7};

//#pragma unroll
    for( int i = 0; i < 8; i++ )
        x[i*sx] = a[reversed[i]].x;
}

inline void storey8( T2 *a, __local T *x, int sx ) {
    int reversed[] = {0,4,2,6,1,5,3,7};

//#pragma unroll
    for( int i = 0; i < 8; i++ )
        x[i*sx] = a[reversed[i]].y;
}


inline void loadx8( T2 *a, __local T *x, int sx ) {
    for( int i = 0; i < 8; i++ )
        a[i].x = x[i*sx];
}

inline void loady8( T2 *a, __local T *x, int sx ) {
    for( int i = 0; i < 8; i++ )
        a[i].y = x[i*sx];
}


#define transpose( a, s, ds, l, dl, sync )                              \
{                                                                       \
    storex8( a, s, ds );  if( (sync)&8 ) barrier(CLK_LOCAL_MEM_FENCE);  \
    loadx8 ( a, l, dl );  if( (sync)&4 ) barrier(CLK_LOCAL_MEM_FENCE);  \
    storey8( a, s, ds );  if( (sync)&2 ) barrier(CLK_LOCAL_MEM_FENCE);  \
    loady8 ( a, l, dl );  if( (sync)&1 ) barrier(CLK_LOCAL_MEM_FENCE);  \
}

inline T2 exp_i( T phi ) {
//#ifdef USE_NATIVE
//    return (T2)( native_cos(phi), native_sin(phi) );
//#else
    return (T2)( cos(phi), sin(phi) );
//#endif
}

inline T2 cmplx_mul( T2 a, T2 b ) { return (T2)( a.x*b.x-a.y*b.y, a.x*b.y+a.y*b.x ); }
inline T2 cm_fl_mul( T2 a, T  b ) { return (T2)( b*a.x, b*a.y ); }
inline T2 cmplx_add( T2 a, T2 b ) { return (T2)( a.x + b.x, a.y + b.y ); }
inline T2 cmplx_sub( T2 a, T2 b ) { return (T2)( a.x - b.x, a.y - b.y ); }


#define twiddle8(a, i, n )                                              \
{                                                                       \
    int reversed8[] = {0,4,2,6,1,5,3,7};                                \
    for( int j = 1; j < 8; j++ ){                                       \
        a[j] = cmplx_mul( a[j],exp_i((-2*M_PI*reversed8[j]/(n))*(i)) ); \
    }                                                                   \
}

#define FFT2(a0, a1)                            \
{                                               \
    T2 c0 = *a0;                           \
    *a0 = cmplx_add(c0,*a1);                    \
    *a1 = cmplx_sub(c0,*a1);                    \
}

#define FFT4(a0, a1, a2, a3)                    \
{                                               \
    FFT2( a0, a2 );                             \
    FFT2( a1, a3 );                             \
    *a3 = cmplx_mul(*a3,exp_1_4);               \
    FFT2( a0, a1 );                             \
    FFT2( a2, a3 );                             \
}

#define FFT8(a)                                                 \
{                                                               \
    FFT2( &a[0], &a[4] );                                       \
    FFT2( &a[1], &a[5] );                                       \
    FFT2( &a[2], &a[6] );                                       \
    FFT2( &a[3], &a[7] );                                       \
                                                                \
    a[5] = cm_fl_mul( cmplx_mul(a[5],exp_1_8) , M_SQRT1_2 );    \
    a[6] =  cmplx_mul( a[6] , exp_1_4);                         \
    a[7] = cm_fl_mul( cmplx_mul(a[7],exp_3_8) , M_SQRT1_2 );    \
                                                                \
    FFT4( &a[0], &a[1], &a[2], &a[3] );                         \
    FFT4( &a[4], &a[5], &a[6], &a[7] );                         \
}

#define itwiddle8( a, i, n )                                            \
{                                                                       \
    int reversed8[] = {0,4,2,6,1,5,3,7};                                \
    for( int j = 1; j < 8; j++ )                                        \
        a[j] = cmplx_mul(a[j] , exp_i((2*M_PI*reversed8[j]/(n))*(i)) ); \
}

#define IFFT2 FFT2

#define IFFT4( a0, a1, a2, a3 )                 \
{                                               \
    IFFT2( a0, a2 );                            \
    IFFT2( a1, a3 );                            \
    *a3 = cmplx_mul(*a3 , iexp_1_4);            \
    IFFT2( a0, a1 );                            \
    IFFT2( a2, a3);                             \
}

#define IFFT8( a )                                              \
{                                                               \
    IFFT2( &a[0], &a[4] );                                      \
    IFFT2( &a[1], &a[5] );                                      \
    IFFT2( &a[2], &a[6] );                                      \
    IFFT2( &a[3], &a[7] );                                      \
                                                                \
    a[5] = cm_fl_mul( cmplx_mul(a[5],iexp_1_8) , M_SQRT1_2 );   \
    a[6] = cmplx_mul( a[6] , iexp_1_4);                         \
    a[7] = cm_fl_mul( cmplx_mul(a[7],iexp_3_8) , M_SQRT1_2 );   \
                                                                \
    IFFT4( &a[0], &a[1], &a[2], &a[3] );                        \
    IFFT4( &a[4], &a[5], &a[6], &a[7] );                        \
}

///////////////////////////////////////////

__kernel void fft1D_512 (__global T2 *work, __local T *smem)
{
  int tid = get_local_id(0);
  int blockIdx = get_group_id(0) * 512 + tid;
  int hi = tid>>3;
  int lo = tid&7;
  T2 data[8];
  // __local T smem[8*8*9];

  // starting index of data to/from global memory
  work = work + blockIdx;
  //out = out + blockIdx;
  globalLoads8(data, work, 64); // coalesced global reads

  FFT8( data );

  twiddle8( data, tid, 512 );
  transpose(data, &smem[hi*8+lo], 66, &smem[lo*66+hi], 8, 0xf);

  FFT8( data );

  twiddle8( data, hi, 64 );
  transpose(data, &smem[hi*8+lo], 8*9, &smem[hi*8*9+lo], 8, 0xE);

  FFT8( data );

  globalStores8(data, work, 64);
}



__kernel void ifft1D_512 (__global T2 *work)
{
  int i;
  int tid = get_local_id(0);
  int blockIdx = get_group_id(0) * 512 + tid;
  int hi = tid>>3;
  int lo = tid&7;
  T2 data[8];
  __local T smem[8*8*9];

  // starting index of data to/from global memory
  work = work + blockIdx;
  globalLoads8(data, work, 64); // coalesced global reads

  // Inject an artificial error for testing the sensitivity of FFT
  // if( blockIdx == 0 ){ data[6] *= 1.001; }

  IFFT8( data );

  itwiddle8( data, tid, 512 );
  transpose(data, &smem[hi*8+lo], 66, &smem[lo*66+hi], 8, 0xf);

  IFFT8( data );

  itwiddle8( data, hi, 64 );
  transpose(data, &smem[hi*8+lo], 8*9, &smem[hi*8*9+lo], 8, 0xE);

  IFFT8( data );

  for(i=0; i<8; i++) {
      data[i].x = data[i].x/512.0f;
      data[i].y = data[i].y/512.0f;
  }

  globalStores8(data, work, 64);

}

__kernel void
chk1D_512(__global T2* work, int half_n_cmplx, __global int* fail)
{
    int i, tid = get_local_id(0);
    int blockIdx = get_group_id(0) * 512 + tid;
    T2 a[8], b[8];

    work = work + blockIdx;

    for (i = 0; i < 8; i++) {
        a[i] = work[i*64];
    }

    for (i = 0; i < 8; i++) {
        b[i] = work[half_n_cmplx+i*64];
    }

    for (i = 0; i < 8; i++) {
        if (a[i].x != b[i].x || a[i].y != b[i].y) {
            *fail = 1;
        }
    }
}
