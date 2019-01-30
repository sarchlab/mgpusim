# Getting Started

## Introduction

In this document, we introduce how to setup the environment to run Akita GCN3 simulator. The purpose is to give developers who are not very familiar with the Go development environment a quick start.

## Install Go

Use this [link](https://golang.org/dl/) to download the Go environment. In this document, we demonstrate how to configure the Linux environment. But the GCN3Sim should be able to run on any platforms, such as Windows and Mac OS X.

By the time we write this document, the most up-to-date version of Go is 1.11.5. Therefore, download the `go1.11.5.linux_amd64.tar.gz` file. You can either follow the installation guide on the Go website or the instructions below.

After downloading the file, you will need to change your work directory to the directory that contains the downloaded tarball file and run the following command.

```bash
sudo tar -C /usr/local -xzf go1.11.5.linux-amd64.tar.gz
```

After installing the packages, you will need to set the PATH environment variable. You can add the following line to your `~/.bashrc` file and run `source ~/.bashrc`.

```bash
export PATH=$PATH:/usr/local/go/bin:~/go
```

## Insalling Akita GCN3 Model

After properly installing Go environment, you should be able to run the following command. It does not matter what is your current working directory.

```bash
go get gitlab.com/akita/gcn3
```

If there is no error runing this command, it should create a folder `~/go` for you. This folder is called `GOPATH` and all the go code should be in this directory. For example the GCN3 model code are located at `~/go/src/gitlab.com/akita/gcn3`.

Dependencies are managed using `go get` commands. To install all the
dependencies, you need to simply run the following command in
`~/go/src/gitlab/akita/gcn3`

```bash
go get ./...
```

The argument `./...` is a go convention that repensents perform some operation recursively in the directory. Therefore, this command will download all the dependencies.

## Run Samples

A set of sample experiments are located in `~/go/src/gitlab.com/akita/gcn3/samples` folder. The sample code configures the platform under simulation, initiates the benchmark, and simulates the benchmark running on the plaform. Akita GCN3 does not provide a main program, but each experiment should have a main program on their own. You would first `cd` into a individual folder and compile the code with:

```bash
go build
```

This command is like `make` for C++ projects. It compiles the the main program as well as the whole simulator into a single binary file. Suppose you are testing the `fir` sample, the binary file should named as `fir`. You can run `./fir -h` for help information, and run the two commands as follow for functional emulation and detailed timing simulation.

```bash
./fir            # For functional emulation
./fir -timing    # For detailed simulation
```