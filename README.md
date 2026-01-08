# MGPUSIM



![GitHub Discussions](https://img.shields.io/github/discussions/sarchlab/mgpusim)


[![MGPUSim Test](https://github.com/sarchlab/mgpusim/actions/workflows/mgpusim_test.yml/badge.svg)](https://github.com/sarchlab/mgpusim/actions/workflows/mgpusim_test.yml)

[![Go Reference](https://pkg.go.dev/badge/github.com/sarchlab/mgpusim.svg)](https://pkg.go.dev/github.com/sarchlab/mgpusim)
[![Go Report Card](https://goreportcard.com/badge/github.com/sarchlab/mgpusim/v4)](https://goreportcard.com/report/github.com/sarchlab/mgpusim/v4)

MGPUSim Documents can be found [here](https://akitasim.dev/docs/mgpusim/intro). Please raise issues if you need documentation on a specific aspect. 


MGPUSim is a high-flexibility, high-performance, high-accuracy GPU simulator. It models GPUs that run the AMD GCN3 instruction sets. One main feature of MGPUSim is the support for multi-GPU simulation (you can still use it for single-GPU architecture research).

## <span style="color:red">⚠️ Important Note on NVIDIA Simulation</span>

<span style="color:red">**Warning**: NVIDIA GPU simulation is under ongoing development and is not ready for use. Currently, only AMD GCN3-based GPU simulation is stable and supported.</span>

## Getting Started

- Install the most recent version of Go from golang.org.
- Clone this repository, assuming the path is `[mgpusim_home]`.
- Change your current directory to `[mgpusim_home]/samples/fir`.
- Compile the simulator with the benchmark with `go build`. The compiler will generate an executable file called `fir` (on Linux or Mac OS) or `fir.exe` (on Windows) for you.
- Run the simulation with `./fir -timing --report-all` to run the simulation.
- Check the generated `.sqlite3` file for high-level metrics output. The metrics are stored in the `mgpusim_metrics` table.

## Develop with Modified Version of Akita (or other depending libraries)

If a modification to Akita is required, you can clone Akita next to the MGPUSim directory in your system. Then, you can modify the `go.mod` file to include the following line. 

```
replace github.com/sarchlab/akita/v4 => ../akita
```

This line will direct the go compiler to use your local version of Akita rather than the official release of Akita. 

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

The metrics are stored in a SQLite database file (`.sqlite3`) in the `mgpusim_metrics` table. You can access the data using any SQLite tool or query it directly:

```bash
# View all metrics
sqlite3 your_simulation_file.sqlite3 "SELECT * FROM mgpusim_metrics;"

# View specific metrics for a component
sqlite3 your_simulation_file.sqlite3 "SELECT * FROM mgpusim_metrics WHERE Location LIKE '%CU%';"
```

## How to Prepare Your Own Experiment

- Create a new repository repo. Typically we create one repo for each project, which may contain multiple experiments.
- Create a folder in your repo for each experiment. Run `go init [git repo path]/[directory_name]` to initialize the folder as a new go module. For example, if your git repository is hosted at `https://github.com/syifan/fancy_project` and your experiment folder is named as `exp1`, your module path should be `github.com/syifan/fancy_project/exp1`.
- Copy all the files under the directory `samples/experiment` to your experiment folder. In the `main.go` file, change the benchmark and the problem size to run. Or you can use an argument to select which benchmark to run. The file `runner.go`, `platform.go`, `r9nano.go`, and `shaderarray.go` serve as configuration files. So you need to change them according to your need.
- It is also possible to modify an existing component or adding a new component. You should copy the folder that includes the component you want to modify to your repo first. Then, modify the configuration scripts to link the system with your new component. You can try to add some print commands to see if your local component is used. Finally, you can start to modify the component code.

## Contributing

- If you find any bug related to the simulator (e.g., simulator is not accurately modeling some behavior or the simulator is not getting the correct emulation result), please raise an issue in the issue tab.
- If you want a new feature (e.g., you need to implement some new instructions or you want to model some new components), please also raise an issue.
- If you want to add a feature or fix a bug, create a pull request.
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
