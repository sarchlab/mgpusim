# MGPUSIM

[![Go Report Card](https://goreportcard.com/badge/gitlab.com/akita/mgpusim)](https://goreportcard.com/report/gitlab.com/akita/mgpusim)
[![Test](https://gitlab.com/akita/mgpusim/badges/master/pipeline.svg)](https://gitlab.com/akita/mgpusim/commits/master)
[![Coverage](https://gitlab.com/akita/mgpusim/badges/master/coverage.svg)](https://gitlab.com/akita/mgpusim/commits/master)

MGPUSim is a high-flexibility, high-performance, high-accuracy GPU simulator. It models GPUs that run the AMD GCN3 instruction sets. One main feature of MGPUSim is the support for multi-GPU simulation (you can still use it for single-GPU architecture research).

## Communication

Slack: [![Slack](https://whispering-taiga-44824.herokuapp.com/badge.svg)](https://join.slack.com/t/projectakita/shared_invite/enQtODEzMDcyNzMyNDUyLWQyMWQyODI2NzIxN2Y5YzYzMTZkZDE3MDk4MzM5MDI2OTY0Yzc4OWFkNjlmZmU3MWJjZmEyNjA0YmNjNTY4Mjk)

Discord: [![Discord Chat](https://img.shields.io/discord/526419346537447424.svg)](https://discord.gg/dQGWq7H)

## Getting Started

- Install the most recent version of Go from golang.org.
- Clone this repository, assuming the path is `[mgpusim_home]`.
- Change your current directory to `[mgpusim_home]/samples/fir`.
- Compile the simulator with the benchmark with `go build`. The compiler will generate an executed called `fir` (on Linux or Mac OS) or `fir.exe` (on Windows) for you.
- Run the simulation with `./fir -timing --report-all` to run the simulation.
- Check the generated `metrics.csv` file for high-level metrics output.

## Benchmark Support

| AMD APP SDK           | DNN Mark   | HeteroMark | Polybench | Rodinia          | SHOC      |
| --------------------- | ---------- | ---------- | --------- | ---------------- | --------- |
| Bitonic Sort          | MaxPooling | AES        | ATAX      | Needleman-Wunsch | BFS       |
| Fast Walsh Transform  | ReLU       | FIR        | BICG      |                  | FFT       |
| Floyd-Warshall        |            | KMeans     |           |                  | SPMV      |
| Matrix Multiplication |            | PageRank   |           |                  | Stencil2D |
| Matrix Transpose      |            |            |           |                  |           |
| NBody                 |            |            |           |                  |           |
| Simple Covolution     |            |            |           |                  |           |

## Default Performance Metrics Supported

You can run a simulation with the `--report-all` argument to enable all the performance metrics.

- Total execution time
- Total kernel time
- Per-GPU kernel time
- Instruction count on each Compute Unit
- Average request latency on all the cache components
- Number of read-misses, read-mshr-hits, read-hits, write-misses, write-mshr-hits, and write hits on all the cache components
- Number of incoming transactions and outgoing transactions on all the RDMA components.
- Number of transactions on each DRAM controller.

## How to Prepare Your Own Experiment

- Create a new repository repo. Typically we create one repo for each project, which may contain multiple experiments.
- Create a folder in your repo for each experiment. Run `go init [git repo path]/[directory_name]` to initialize the folder as a new go module. For example, if your git repository is hosted at `https://gitlab.com/syifan/fancy_project` and your experiment folder is named as `exp1`, your module path should be `gitlab.com/syifan/fancy_project/exp1`.
- Copy all the files under the directory `samples/experiment` to your experiment folder. In the `main.go` file, change the benchmark and the problem size to run. Or you can use an argument to select which benchmark to run. The file `runner.go`, `platform.go`, `r9nano.go`, and `shaderarray.go` serve as configuration files. So you need to change them according to your need.
- It is also possible to modify an existing component or adding a new component. You should copy the folder that includes the component you want to modify to your repo first. Then, modify the configuration scripts to link the system with your new component. You can try to add some print commands to see if your local component is used. Finally, you can start to modify the component code.

## Contributing

- If you find any bug related to the simulator (e.g., simulator is not accurately modeling some behavior or the simulator is not getting the correct emulation result), please raise an issue in the issue tab MGPUSim.
- If you want a new feature (e.g., you need to implement some new instructions or you want to model some new components), please also raise an issue.
- If you want to add a feature or fix a bug, create a merge request using the "Create merge request" button in the corresponding issue. Gitlab will create a branch for you and you can develop your code there. Feel free to commit often and push often as you do not need to be responsible for the coding quality of every commit.
- When you are done with developing, click the "Mark as ready" button in the merge request. Someone will review your code and see if the code can be merged. If nobody responds you in 2 days, please notify us on Slack.
- There is no particular style requirement other than the default Go style requirement. Please run `gofmt`, `goimports`, or `goreturns` before making your merge request ready. Also, running `golangci-lint run` in the root directory will point you out most of the styling errors.

## Citation

If you use MGPUSim in your research, please cite our ISCA '19 paper. 

```bibtex
@inproceedings{sun19mgpusim, 
    author = {Sun, Yifan and Baruah, Trinayan and Mojumder, Saiful A. and Dong, Shi and Gong, Xiang and Treadway, Shane and Bao, Yuhui and Hance, Spencer and McCardwell, Carter and Zhao, Vincent and Barclay, Harrison and Ziabari, Amir Kavyan and Chen, Zhongliang and Ubal, Rafael and Abell\'{a}n, Jos\'{e} L. and Kim, John and Joshi, Ajay and Kaeli, David}, 
    title = {MGPUSim: Enabling Multi-GPU Performance Modeling and Optimization}, 
    year = {2019}, 
    isbn = {9781450366694}, 
    publisher = {Association for Computing Machinery}, 
    address = {New York, NY, USA}, 
    url = {https://doi.org/10.1145/3307650.3322230}, 
    doi = {10.1145/3307650.3322230}, 
    booktitle = {Proceedings of the 46th International Symposium on Computer Architecture}, 
    pages = {197–209}, 
    numpages = {13}, 
    keywords = {simulation, multi-GPU systems, memory management}, 
    location = {Phoenix, Arizona}, 
    series = {ISCA '19} 
}
```

Papers that use MGPUSim:

* Dynamic GMMU Bypass for Address Translation in Multi-GPU Systems
* Valkyrie: Leveraging Inter-TLB Locality to Enhance GPU Performance
* MGPU-TSM: A Multi-GPU System with Truly Shared Memory
* Griffin: Hardware-Software Support for Efficient Page Migration in Multi-GPU Systems
* HALCONE: A Hardware-Level Timestamp-based Cache Coherence Scheme for Multi-GPU systems
* Priority-Based PCIe Scheduling for Multi-Tenant Multi-GPU Systems
* Exploiting Adaptive Data Compression to Improve Performance and Energy-efficiency of Compute Workloads in Multi-GPU Systems


## License

MIT © Project Akita Developers.
