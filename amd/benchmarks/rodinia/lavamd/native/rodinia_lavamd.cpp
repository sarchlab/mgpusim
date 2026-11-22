// GPU Benchmarks v1.1, tier2/rodinia_lavamd/hip/rodinia_lavamd.hip.
#include <hip/hip_runtime.h>
#include <cmath>

#define fp double

#define NUMBER_PAR_PER_BOX 100          // keep low so more blocks share SM
#define NUMBER_THREADS 128              // ~ NUMBER_PAR_PER_BOX for best perf

#define DOT(A,B) ((A.x)*(B.x)+(A.y)*(B.y)+(A.z)*(B.z))  // STABLE

typedef struct { fp x, y, z; } THREE_VECTOR;
typedef struct { fp v, x, y, z; } FOUR_VECTOR;

typedef struct nei_str {
    int x, y, z;
    int number;
    long offset;
} nei_str;

typedef struct box_str {
    int x, y, z;
    int number;
    long offset;
    int nn;
    nei_str nei[26];
} box_str;

typedef struct par_str {
    fp alpha;
} par_str;

typedef struct dim_str {
    int cur_arg;
    int arch_arg;
    int cores_arg;
    int boxes1d_arg;
    long number_boxes;
    long box_mem;
    long space_elem;
    long space_mem;
    long space_mem2;
} dim_str;

// ---------------------------------------------------------------------------
// KERNEL — verbatim from Rodinia lavaMD/kernel/kernel_gpu_cuda.cu
// ---------------------------------------------------------------------------
extern "C" __global__ void kernel_gpu_cuda(par_str d_par_gpu,
								dim_str d_dim_gpu,
								box_str* d_box_gpu,
								FOUR_VECTOR* d_rv_gpu,
								fp* d_qv_gpu,
								FOUR_VECTOR* d_fv_gpu)
{

	int bx = blockIdx.x;
	int tx = threadIdx.x;
	int wtx = tx;

	if(bx<d_dim_gpu.number_boxes){

		// parameters
		fp a2 = 2.0*d_par_gpu.alpha*d_par_gpu.alpha;

		// home box
		int first_i;
		FOUR_VECTOR* rA;
		FOUR_VECTOR* fA;
		__shared__ FOUR_VECTOR rA_shared[100];

		// nei box
		int pointer;
		int k = 0;
		int first_j;
		FOUR_VECTOR* rB;
		fp* qB;
		int j = 0;
		__shared__ FOUR_VECTOR rB_shared[100];
		__shared__ double qB_shared[100];

		// common
		fp r2;
		fp u2;
		fp vij;
		fp fs;
		fp fxij;
		fp fyij;
		fp fzij;
		THREE_VECTOR d;

		// home box - box parameters
		first_i = d_box_gpu[bx].offset;

		// home box - distance, force, charge and type parameters
		rA = &d_rv_gpu[first_i];
		fA = &d_fv_gpu[first_i];

		// home box - shared memory
		while(wtx<NUMBER_PAR_PER_BOX){
			rA_shared[wtx] = rA[wtx];
			wtx = wtx + NUMBER_THREADS;
		}
		wtx = tx;

		// synchronize threads  - not needed, but just to be safe
		__syncthreads();

		// loop over neiing boxes of home box
		for (k=0; k<(1+d_box_gpu[bx].nn); k++){

			//----------------------------------------50
			//	nei box - get pointer to the right box
			//----------------------------------------50

			if(k==0){
				pointer = bx;													// set first box to be processed to home box
			}
			else{
				pointer = d_box_gpu[bx].nei[k-1].number;							// remaining boxes are nei boxes
			}

			// nei box - box parameters
			first_j = d_box_gpu[pointer].offset;

			// nei box - distance, (force), charge and (type) parameters
			rB = &d_rv_gpu[first_j];
			qB = &d_qv_gpu[first_j];

			// nei box - shared memory
			while(wtx<NUMBER_PAR_PER_BOX){
				rB_shared[wtx] = rB[wtx];
				qB_shared[wtx] = qB[wtx];
				wtx = wtx + NUMBER_THREADS;
			}
			wtx = tx;

			// synchronize threads because in next section each thread accesses data brought in by different threads here
			__syncthreads();

			// loop for the number of particles in the home box
			while(wtx<NUMBER_PAR_PER_BOX){

				// loop for the number of particles in the current nei box
				for (j=0; j<NUMBER_PAR_PER_BOX; j++){

					r2 = (fp)rA_shared[wtx].v + (fp)rB_shared[j].v - DOT((fp)rA_shared[wtx],(fp)rB_shared[j]);
					u2 = a2*r2;
					vij= exp(-u2);
					fs = 2*vij;

					d.x = (fp)rA_shared[wtx].x  - (fp)rB_shared[j].x;
					fxij=fs*d.x;
					d.y = (fp)rA_shared[wtx].y  - (fp)rB_shared[j].y;
					fyij=fs*d.y;
					d.z = (fp)rA_shared[wtx].z  - (fp)rB_shared[j].z;
					fzij=fs*d.z;

					fA[wtx].v +=  (double)((fp)qB_shared[j]*vij);
					fA[wtx].x +=  (double)((fp)qB_shared[j]*fxij);
					fA[wtx].y +=  (double)((fp)qB_shared[j]*fyij);
					fA[wtx].z +=  (double)((fp)qB_shared[j]*fzij);

				}

				// increment work thread index
				wtx = wtx + NUMBER_THREADS;

			}

			// reset work index
			wtx = tx;

			// synchronize after finishing force contributions from current nei box not to cause conflicts when starting next box
			__syncthreads();

		}

	}

}
