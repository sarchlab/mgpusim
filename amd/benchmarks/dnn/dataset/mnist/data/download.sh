#!/bin/bash

wget http://yann.lecun.com/exdb/mnist/train-images-idx3-ubyte.gz \
    --output-document train-images-idx3-ubyte.gz
wget http://yann.lecun.com/exdb/mnist/train-labels-idx1-ubyte.gz \
    --output-document train-labels-idx1-ubyte.gz
wget http://yann.lecun.com/exdb/mnist/t10k-images-idx3-ubyte.gz \
    --output-document t10k-images-idx3-ubyte.gz
wget http://yann.lecun.com/exdb/mnist/t10k-labels-idx1-ubyte.gz \
    --output-document t10k-labels-idx1-ubyte.gz
