#!/bin/bash

export AMDAPPSDKROOT=~/AMDAPPSDK-3.0
rm -rf bin
rm -rf build
mkdir build
cd build
cmake ..
make
