#!/bin/bash

set -e

cd samples/firsim
go build
./firsim -verify -dataSize-8192
./firsim -timing -verify -dataSize=8192
./firsim -parallel -verify -dataSize=8192
./firsim -timing -parallel -verify -dataSize=8192
cd ../../
