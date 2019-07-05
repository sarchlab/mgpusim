#!/bin/bash

export AMDAPPSDKROOT=~/AMDAPPSDK-3.0
mkdir build
cd build
cmake ..
make
