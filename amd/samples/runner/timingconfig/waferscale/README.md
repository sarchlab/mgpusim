# Wafer-scale GPU

`waferscale` configures a baseline wafer-scale GPU for timing simulation: GPU
tiles on a 2D mesh with the host in the centre. The baseline is Model 1 of
Akkalat, a 5x5 mesh whose 24 non-host positions each hold an R9 Nano-based
tile of 32 CUs and 4 GiB of memory. It is selected like the other GPU types,
`r9nano` and `mi300x`.

## Quick start

```bash
cd amd/samples/fir && go build
./fir -timing -gpu waferscale -verify                     # 5x5 wafer, runs on GPU[1]
./fir -timing -gpu waferscale -gpus 1,24 -verify          # two opposite corners
./fir -timing -gpu waferscale -wafer-mesh 7x7 -gpus 1,48  # 48 tiles
./fir -timing -gpu waferscale -wafer-config-html wafer.html
```

The last command writes the configuration page and exits without running the
benchmark (see [Configuration page](#configuration-page)).

| Flag | Default | Meaning |
|---|---|---|
| `-gpu waferscale` | `r9nano` | select the wafer-scale GPU; `waferscale-<tile>` names a tile microarchitecture, and `waferscale` is `waferscale-r9nano` |
| `-wafer-mesh WxH` | `5x5` | mesh shape; a WxH mesh has W*H-1 tiles |
| `-gpus 1,2,...` | `1` | the tiles a benchmark runs on, numbered from 1 |
| `-wafer-config-html PATH` | none | write the configuration page to PATH and exit |

The wafer always has every tile of its mesh, whatever `-gpus` selects. A
benchmark uses only the tiles that `-gpus` (or `-unified-gpus`) lists; the
others are idle.

From Go:

```go
runtime := timingconfig.MakeBuilder().
    WithSimulation(s).
    WithGPUType("waferscale").
    WithWaferScaleMesh(7, 7). // optional, 5x5 by default
    Build()
```

`WithNumGPUs` has no effect on a wafer-scale GPU: the mesh sets the number of
GPUs.

## Topology

The host, which holds the driver, the IOMMU, and the CPU memory, sits at
`(W/2, H/2)`. Tiles fill the mesh row by row from `(0, 0)` and skip the host,
the order Akkalat creates them in. On the 5x5 baseline:

```
        x=0   x=1   x=2   x=3   x=4
 y=0      1     2     3     4     5
 y=1      6     7     8     9    10
 y=2     11    12   HOST   13    14
 y=3     15    16    17    18    19
 y=4     20    21    22    23    24
```

On a mesh with an even side, the host takes the position just below and to the
right of the centre, `(2, 2)` on a 4x4 mesh.

Every port that the GPUs and the host expose to each other joins the endpoint
of its device's mesh position: the command processor, RDMA, page migration,
data mover, access counter, and page-table walker (GMMU) ports of each tile,
and the driver, IOMMU, and CPU memory ports of the host. Messages route in Y first,
then X (`akita/noc/mesh` routes Z, then Y, then X). A tile is
`|x - W/2| + |y - H/2|` hops from the host; the corners of the 5x5 baseline are
4 hops away.

## Parameters

### Tile

| Parameter | R9 Nano tile |
|---|---|
| Shader arrays | 8 |
| CUs per shader array | 4 |
| CUs | 32 |
| L2 cache | 4 MiB |
| DRAM | 4 GiB |
| Page size | 4 KiB |

The tile is the `r9nano` builder with these four parameters changed; every
other parameter is that of `r9nano`.

### Mesh

| Parameter | Value |
|---|---|
| Frequency | 1 GHz |
| Flit size | 16 B |
| Input and output width | 1 flit per cycle |
| Link bandwidth | 16 GB/s (16 B x 1 GHz), Akkalat's default `-bandwidth 1` |
| Switch latency | 20 cycles between switches, 1 cycle on device ports |

The mesh models ideal links: switch processing and buffering limit throughput,
but there is no physical link bandwidth or propagation latency (see the
`akita/noc/mesh` README).

### Memory and translation

GPU `i` owns the physical addresses `[i x 4 GiB, (i+1) x 4 GiB)`; the CPU
memory owns the first 4 GiB. The RDMA engines route remote accesses by these
4 GiB ranges.

The IOMMU keeps its page tables in a pool of 4 KiB nodes at the start of the
CPU memory. The pool grows with the memory it must map: 8 bytes of leaf PTE
per page, plus one third for the upper levels and some sparsity.

| Mesh | Tiles | Mapped memory | IOMMU pool |
|---|---|---|---|
| 3x1 | 2 | 8 GiB | 16 384 nodes (64 MiB, the default minimum) |
| 4x4 | 15 | 60 GiB | 40 960 nodes (160 MiB) |
| 5x5 | 24 | 96 GiB | 65 536 nodes (256 MiB) |
| 7x7 | 48 | 192 GiB | 131 072 nodes (512 MiB) |

A pool set explicitly with `WithPageTable` is kept. A mesh whose pool does not
fit in the 4 GiB of CPU memory panics when it is built.

## Configuration page

`-wafer-config-html PATH` writes one self-contained HTML file that describes
the platform as built, read from the `Spec` of every component the way Akita's
topology recorder reads it, so the page shows what the simulation runs, not a
separate list of parameters. It needs `-timing -gpu waferscale`.

- The first level is the wafer: the mesh with the host in the centre, and the
  totals for the wafer, each tile, and the mesh network.
- Clicking a tile opens its GPU, the second level: its components grouped by
  kind, such as `SA[].CU[] x32`. Each kind lists the fields that all its
  components share once, and the fields that differ, which are the ports each
  component sends to, per component.
- The Host & Network tab lists the driver, the IOMMU, the CPU memory, and the
  mesh switches and endpoints.
- The filter box finds components and fields by name or value, for example
  `latency`.

The page follows the look of AkitaRTM. It is about 1.7 MB for 5x5 and 3.6 MB
for 7x7. It needs no network, except for the Asap font, which falls back to
the system font offline.

## Differences from Akkalat

Akkalat is built on Akita v3 and MGPUSim v3; this package is a rewrite on
Akita v5, not a copy.

| | Akkalat | This package |
|---|---|---|
| Tile memory | 8 GiB | 4 GiB, as in the issue |
| IOMMU | IOMMU TLB and IOMMU cache in front of a 500-cycle MMU | the Akita v5 page-table IOMMU (`pagetablemmu`) |
| `-bandwidth` | sets the link bandwidth | not available: the v5 mesh has no link bandwidth |
| `-max-num-hops` | limits the hops of a flit, -1 (no limit) by default | not available: the v5 mesh has no such limit |
| Mesh size | 7x7 in the code, 5x5 as Model 1 | 5x5 by default, any size with `-wafer-mesh` |

## Limitations

- The only tile microarchitecture is R9 Nano. An MI300X tile
  (`waferscale-mi300x`) is not implemented yet: the MI300X builder derives its
  shader arrays from its XCDs, and shrinking it to a 32-CU tile is untested.
- The tests and the FIR runs above check that the wafer builds, that its routes
  are valid, and that work on one or several tiles finishes correctly. FIR
  barely moves data between tiles, so no test yet exercises inter-tile RDMA or
  page migration across the mesh.
- The host-transfer latencies are those of `r9nano`.

## Code

| File | Contents |
|---|---|
| `waferscale/builder.go` | mesh shape, tile locations, tile parameters, the R9 Nano tile builder, and the IOMMU pool size |
| `waferscale/configpage.go` | collecting the component specs and writing the configuration page |
| `waferscale/configpage.html.tmpl` | the page template, embedded in the binary |
| `../network.go` | the inter-device network: a direct connection for `r9nano` and `mi300x`, the mesh for `waferscale` |
| `../builder.go` | the `waferscale` GPU type in `timingconfig.Builder` and `WithWaferScaleMesh` |
| `amd/samples/sample/flag.go`, `waferconfig.go` | the `-wafer-mesh` and `-wafer-config-html` flags |

The template is a `.tmpl` file because `amd/.gitignore` ignores `*.html`, the
generated pages.

## Tests

```bash
go test ./amd/samples/runner/timingconfig/... -run 'WaferScale|Flatten|Split'
go test ./amd/samples/sample/ -run 'WaferConfig|MeshShape'
```

The platform tests check that the 5x5 and 7x7 wafers build with valid routes,
one mesh switch per position, the 65 536-node IOMMU pool of the baseline, and a
configuration page that places every component on the host, the mesh, or its
tile.
