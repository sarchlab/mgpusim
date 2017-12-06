# Getting Started

## Introduction

In this document, we introduce how to setup the environment to run GCN3Sim. The purpose is to give developers who are not very familiar with the GO development environment a quick start.

## Install Go

Use this [link](https://golang.org/dl/) to download the GO development environment. In this document, we demonstrate how to configure the Linux environment. But the GCN3Sim should be able to run on any platforms, such as Windows and Mac OS X.

By the time we write this document, the most up-to-date version of go is 1.9.2. Therefore, download the `go1.9.2.linux_amd64.tar.gz` file and follow the instruction on the website to install go. After downloading the file, you will need to change your work directory to the directory that contains the downloaded tarball file and run the following command. You may need to use sudo to grant the permission to install the packages.

```bash
tar -C /usr/local -xzf go1.9.2.linux-amd64.tar.gz
```

After installing the packages, you will need to set the PATH environment
variable to be able to run the `go` executable file. You can add the following
line to your `$HOME/.bashrc` file and run `source .bashrc`.

```bash
export PATH=$PATH:/usr/local/go/bin
```

## Insalling GCN3

After properly installing Go environment, you should be able to run the following command. It does not matter what is your current working directory.

```bash
go get gitlab.com/yaotsu/gcn3
```

If there is no error runing this command, it should create a folder `$HOME/go`
for you. This folder is called `GOPATH` and all the go code should be in this.
directory. For example the GCN3Sim code are located at
`$HOME/go/src/gitlab.com/yaotsu/gcn3`.

If you are working on another branch of the GCN3Sim, this is a good time to
switch to the branch you want to work on.

## Installing Dependencies

Dependencies are managed using `go get` commands. To install all the
dependencies, you need to simply run the following command in
`$HOME/go/src/gitlab/yaotsu/gcn3`

```bash
go get -t ./...
```

The arguement `-t` will also download the dependencies required for not only
executing GCN3Sim, but also testing GCN3Sim. The argument `./...` is a go
convention that repensents perform some operation recursively in the directory.
Therefore, this command will download all the dependencies, even for the
subpackages of the GCN3Sim.

GCN3Sim uses [ginkgo](https://github.com/onsi/ginkgo) as its unit test
framework. To run unit tests, you would still need to install ginkgo manually
by running

```bash
go get github.com/onsi/ginkgo/ginkgo
```

Once you have `ginkgo` installed, command `gingko -r` will discover and run all
the unit tests in GCN3Sim and its subpackages.

## Run Samples

TODO