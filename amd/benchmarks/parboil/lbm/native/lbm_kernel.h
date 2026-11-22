// (C) Copyright 2010 The Board of Trustees of the University of Illinois.
// Adapted from Parboil a4c63da28f45699e9a274cbc49af3e067162aa2b.
// See UPSTREAM_LICENSE and README.md for provenance and adaptations.
#pragma once
#include "lbm_common.h"
namespace lbm {
extern "C" __global__ void lbm_collide_stream_kernel(const float* srcGrid,float* dstGrid,Layout l) {
    const int cell=threadIdx.x+blockIdx.x*l.px+blockIdx.y*l.plane;
    const unsigned cell_flags=__float_as_uint(srcGrid[FLAGS*l.cells+cell]);
    constexpr float DFL1=1.0f/3.0f,DFL2=1.0f/18.0f,DFL3=1.0f/36.0f;
	float temp_swp, tempC, tempN, tempS, tempE, tempW, tempT, tempB;
	float tempNE, tempNW, tempSE, tempSW, tempNT, tempNB, tempST ;
	float tempSB, tempET, tempEB, tempWT, tempWB ;

	//Load all of the input fields
	//This is a gather operation of the SCATTER preprocessor variable
        // is undefined in layout_config.h, or a "local" read otherwise
	tempC = srcGrid[C*l.cells+cell];
	tempN = srcGrid[N*l.cells+cell];
	tempS = srcGrid[S*l.cells+cell];
	tempE = srcGrid[E*l.cells+cell];
	tempW = srcGrid[W*l.cells+cell];
	tempT = srcGrid[T*l.cells+cell];
	tempB = srcGrid[B*l.cells+cell];
	tempNE= srcGrid[NE*l.cells+cell];
	tempNW= srcGrid[NW*l.cells+cell];
	tempSE = srcGrid[SE*l.cells+cell];
	tempSW = srcGrid[SW*l.cells+cell];
	tempNT = srcGrid[NT*l.cells+cell];
	tempNB = srcGrid[NB*l.cells+cell];
	tempST = srcGrid[ST*l.cells+cell];
	tempSB = srcGrid[SB*l.cells+cell];
	tempET = srcGrid[ET*l.cells+cell];
	tempEB = srcGrid[EB*l.cells+cell];
	tempWT = srcGrid[WT*l.cells+cell];
	tempWB = srcGrid[WB*l.cells+cell];

	//Test whether the cell is fluid or obstacle
	if( (cell_flags & OBSTACLE)) {
		//Swizzle the inputs: reflect any fluid coming into this cell
		// back to where it came from
		temp_swp = tempN ; tempN = tempS ; tempS = temp_swp ;
		temp_swp = tempE ; tempE = tempW ; tempW = temp_swp;
		temp_swp = tempT ; tempT = tempB ; tempB = temp_swp;
		temp_swp = tempNE; tempNE = tempSW ; tempSW = temp_swp;
		temp_swp = tempNW; tempNW = tempSE ; tempSE = temp_swp;
		temp_swp = tempNT ; tempNT = tempSB ; tempSB = temp_swp;
		temp_swp = tempNB ; tempNB = tempST ; tempST = temp_swp;
		temp_swp = tempET ; tempET= tempWB ; tempWB = temp_swp;
		temp_swp = tempEB ; tempEB = tempWT ; tempWT = temp_swp;
	}
	else {
                //The math meat of LBM: ignore for optimization
	        float ux, uy, uz, rho, u2;
		float temp1, temp2, temp_base;
		rho = tempC + tempN
			+ tempS + tempE
			+ tempW + tempT
			+ tempB + tempNE
			+ tempNW + tempSE
			+ tempSW + tempNT
			+ tempNB + tempST
			+ tempSB + tempET
			+ tempEB + tempWT
			+ tempWB;

		ux = + tempE - tempW
			+ tempNE - tempNW
			+ tempSE - tempSW
			+ tempET + tempEB
			- tempWT - tempWB;
		uy = + tempN - tempS
			+ tempNE + tempNW
			- tempSE - tempSW
			+ tempNT + tempNB
			- tempST - tempSB;
		uz = + tempT - tempB
			+ tempNT - tempNB
			+ tempST - tempSB
			+ tempET - tempEB
			+ tempWT - tempWB;

		ux /= rho;
		uy /= rho;
		uz /= rho;
		if( (cell_flags & ACCEL)) {
			ux = 0.005f;
			uy = 0.002f;
			uz = 0.000f;
		}
		u2 = 1.5f * (ux*ux + uy*uy + uz*uz) - 1.0f;
		temp_base = omega*rho;
		temp1 = DFL1*temp_base;


		//Put the output values for this cell in the shared memory
		temp_base = omega*rho;
		temp1 = DFL1*temp_base;
		temp2 = 1.0f-omega;
		tempC = temp2*tempC + temp1*(                                 - u2);
	        temp1 = DFL2*temp_base;
		tempN = temp2*tempN + temp1*(       uy*(4.5f*uy       + 3.0f) - u2);
		tempS = temp2*tempS + temp1*(       uy*(4.5f*uy       - 3.0f) - u2);
		tempT = temp2*tempT + temp1*(       uz*(4.5f*uz       + 3.0f) - u2);
		tempB = temp2*tempB + temp1*(       uz*(4.5f*uz       - 3.0f) - u2);
		tempE = temp2*tempE + temp1*(       ux*(4.5f*ux       + 3.0f) - u2);
		tempW = temp2*tempW + temp1*(       ux*(4.5f*ux       - 3.0f) - u2);
		temp1 = DFL3*temp_base;
		tempNT= temp2*tempNT + temp1 *( (+uy+uz)*(4.5f*(+uy+uz) + 3.0f) - u2);
		tempNB= temp2*tempNB + temp1 *( (+uy-uz)*(4.5f*(+uy-uz) + 3.0f) - u2);
		tempST= temp2*tempST + temp1 *( (-uy+uz)*(4.5f*(-uy+uz) + 3.0f) - u2);
		tempSB= temp2*tempSB + temp1 *( (-uy-uz)*(4.5f*(-uy-uz) + 3.0f) - u2);
		tempNE = temp2*tempNE + temp1 *( (+ux+uy)*(4.5f*(+ux+uy) + 3.0f) - u2);
		tempSE = temp2*tempSE + temp1 *((+ux-uy)*(4.5f*(+ux-uy) + 3.0f) - u2);
		tempET = temp2*tempET + temp1 *( (+ux+uz)*(4.5f*(+ux+uz) + 3.0f) - u2);
		tempEB = temp2*tempEB + temp1 *( (+ux-uz)*(4.5f*(+ux-uz) + 3.0f) - u2);
		tempNW = temp2*tempNW + temp1 *( (-ux+uy)*(4.5f*(-ux+uy) + 3.0f) - u2);
		tempSW = temp2*tempSW + temp1 *( (-ux-uy)*(4.5f*(-ux-uy) + 3.0f) - u2);
		tempWT = temp2*tempWT + temp1 *( (-ux+uz)*(4.5f*(-ux+uz) + 3.0f) - u2);
		tempWB = temp2*tempWB + temp1 *( (-ux-uz)*(4.5f*(-ux-uz) + 3.0f) - u2);
	}

	//Write the results computed above
	//This is a scatter operation of the SCATTER preprocessor variable
        // is defined in layout_config.h, or a "local" write otherwise
	dstGrid[C*l.cells+cell+0+0*l.px+0*l.plane] = tempC;

	dstGrid[N*l.cells+cell+0+1*l.px+0*l.plane] = tempN;
	dstGrid[S*l.cells+cell+0-1*l.px+0*l.plane] = tempS;
	dstGrid[E*l.cells+cell+1+0*l.px+0*l.plane] = tempE;
	dstGrid[W*l.cells+cell-1+0*l.px+0*l.plane] = tempW;
	dstGrid[T*l.cells+cell+0+0*l.px+1*l.plane] = tempT;
	dstGrid[B*l.cells+cell+0+0*l.px-1*l.plane] = tempB;

	dstGrid[NE*l.cells+cell+1+1*l.px+0*l.plane] = tempNE;
	dstGrid[NW*l.cells+cell-1+1*l.px+0*l.plane] = tempNW;
	dstGrid[SE*l.cells+cell+1-1*l.px+0*l.plane] = tempSE;
	dstGrid[SW*l.cells+cell-1-1*l.px+0*l.plane] = tempSW;
	dstGrid[NT*l.cells+cell+0+1*l.px+1*l.plane] = tempNT;
	dstGrid[NB*l.cells+cell+0+1*l.px-1*l.plane] = tempNB;
	dstGrid[ST*l.cells+cell+0-1*l.px+1*l.plane] = tempST;
	dstGrid[SB*l.cells+cell+0-1*l.px-1*l.plane] = tempSB;
	dstGrid[ET*l.cells+cell+1+0*l.px+1*l.plane] = tempET;
	dstGrid[EB*l.cells+cell+1+0*l.px-1*l.plane] = tempEB;
	dstGrid[WT*l.cells+cell-1+0*l.px+1*l.plane] = tempWT;
	dstGrid[WB*l.cells+cell-1+0*l.px-1*l.plane] = tempWB;
}
} // namespace lbm
