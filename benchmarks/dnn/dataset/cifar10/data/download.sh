#!/bin/bash

curl https://www.cs.toronto.edu/~kriz/cifar-10-binary.tar.gz \
    --output cifar-10-batches-bin.gz

tar xvzf cifar-10-batches-bin.gz