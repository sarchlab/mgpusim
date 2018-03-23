__kernel void microbench(__global int* sum){
	 atomic_add(sum,1);
}
