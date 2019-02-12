# Getting Started

## Introduction

In this document, we introduce how to setup the environment to run Akita GCN3 simulator. The purpose is to give developers who are not very familiar with the Go development environment a quick start.

## Install Go

Use this [link](https://golang.org/dl/) to download the Go environment. In this document, we demonstrate how to configure the Linux environment. But the GCN3Sim should be able to run on any platforms, including Windows and Mac OS X.

By the time we write this document, the most up-to-date version of Go is 1.11.5. Therefore, download the `go1.11.5.linux_amd64.tar.gz` file. You can either follow the installation guide on the Go website or the instructions below.

After downloading the file, you will need to change your work directory to the directory that contains the downloaded tarball file and run the following command.

```bash
sudo tar -C /usr/local -xzf go1.11.5.linux-amd64.tar.gz
```

After installing the packages, you will need to set the PATH environment variable. You can add the following line to your `~/.bashrc` file and run `source ~/.bashrc`.

```bash
export PATH=$PATH:/usr/local/go/bin:~/go
```

## Installing Akita GCN3 Model

After properly installing Go environment, you should be able to run the following command. It does not matter what is your current working directory.

```bash
go get gitlab.com/akita/gcn3
```

If there is no error running this command, it should create a folder `~/go` for you. This folder is called `GOPATH` and all the go code should be in this directory. For example the GCN3 model code are located at `~/go/src/gitlab.com/akita/gcn3`.

## Run Samples

A set of sample experiments are located in `~/go/src/gitlab.com/akita/gcn3/samples` folder. Suppose we want to run the FIR benchmark, we can `cd` into the `fir` folder and run:

```bash
go build
```

This command would download all the dependencies and compile the the experiment. The output binary file should named as `fir`. You can run `./fir -h` for help information, and run the two commands as follow for functional emulation and detailed timing simulation.

```bash
./fir            # For functional emulation
./fir -timing    # For detailed simulation
```