// (C) Copyright 2010 The Board of Trustees of the University of Illinois.
// Adapted from Parboil a4c63da28f45699e9a274cbc49af3e067162aa2b.
// See UPSTREAM_LICENSE for the Illinois Open Source License.
#pragma once
#include <algorithm>
#include <climits>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <stdexcept>
#include <vector>
namespace lbm {
enum { C,N,S,E,W,T,B,NE,NW,SE,SW,NT,NB,ST,SB,ET,EB,WT,WB,FLAGS,Q=19 };
enum { OBSTACLE=1, ACCEL=2 };
constexpr float omega=1.95f;
constexpr double population_tolerance=1e-5, velocity_tolerance=1e-5;
struct Layout {
    int n,px,plane,cells,margin,entries;
    explicit Layout(int size):n(size),px(n+8),plane(px*n),cells(plane*(n+4)),
        margin(2*plane),entries(20*cells+40*margin) {}
    size_t bytes() const {return size_t(entries)*sizeof(float);}
    int index(int x,int y,int z,int q) const {return margin+q*cells+x+y*px+z*plane;}
};

} // namespace lbm
