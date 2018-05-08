#!/bin/bash
# set -e

cd samples/firsim
go build
./firsim -verify -dataSize=8192
./firsim -timing -verify -dataSize=8192
./firsim -parallel -verify -dataSize=8192
./firsim -timing -parallel -verify -dataSize=8192
cd ../../

cd samples/kmeanssim
go build
./kmeanssim -verify -points=1024 -features=32 -clusters 5
./kmeanssim -timing -verify -points=1024 -features=32 -clusters 5
./kmeanssim -parallel -verify -points=1024 -features=32 -clusters 5
./kmeanssim -timing -verify -points=1024 -features=32 -clusters 5
cd ../../

