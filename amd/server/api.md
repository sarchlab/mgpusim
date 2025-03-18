# MGPUSim Server API

## Device Count

### EndPoint:

**GET** /device_count

### Return Data

```json
{
  "device_count": 4
}
```

## Device Properties

### EndPoint:

**GET** /device_properties/[device_id]

### Return Data

```json
{
  "name": "gfx803",
  "total_global_mem": 4294967296,
  "shared_mem_per_block": 4096,
  "regs_per_block": 102,
  "warp_size": 64,
  "max_threads_per_block": 1024,
  "max_threads_dim": [1024, 256, 64],
  "max_grid_size": [1048576, 65536, 1024],
  "clock_rate": 1048576,
  "mem_clock_rate": 1048576,
  "memory_bus_width": 4096,
  "total_const_mem": 1048576,
  "major": 8,
  "minor": 3,
  "multi_processor_count": 64,
  "l2_cache_size": 2097152,
  "max_threads_per_multi_processor": 2560,
  "compute_mode": 1, // I do not know what it is.
  "clock_instruction_rate": 1048576,
  "arch": {
    "has_global_int32_atomics": true
  },
  "concurrent_kernels": 1,
  "pci_bus_id": 0,
  "pci_deviceid": 0,
  "max_shared_memory_per_multi_processor": 65536,
  "is_multi_gpu_board": 0,
  "can_map_host_memory": 0,
  "gcn_arch": 803
}
```

### Error

- Device is not available
  > 404

## Malloc

### End Point:

**GET** /malloc

### Input Argument

```json
{
  "size": 1024
}
```

### Return Data

```json
{
  "ptr": 12345678
}
```

### Error

- Input size if not given

  > 400

- Input size is negative or 0

  > 400

- Out of memory

  > 400

## Free

### End Point

**GET** /free/[ptr]

### Return Data

```json
{}
```

### Error

- Address is not allocated

  > 400

## Memcopy Host to Device

### End Point

**POST** /memcopy_h2d

### Input Data

```json
{
  "ptr": 4096,
  "data": "[Base64_encoded_binary_data]"
}
```

### Return Data

```json
{}
```

### Error

- Address is not allocated

  > 400

## Memcopy Device to Host

### End Point

**GET** /memcopy_d2h

### Input Data

```json
{
  "ptr": 4096,
  "size": 1024
}
```

### Return Data

```json
{
  "data": "[Base64_encoded_binary_data]"
}
```

### Error

- Address is not allocated

  > 400

## Launch Kernel

### End Point

**POST** /launch_kernel

### Input Data

```json
{
  "code_object": "[Base64 encoded binary data. The first 256 bytes are the HSA Code Object header.]",
  "args": "[Base64 encoded kernel argument data.]",
  "num_blocks": { "x": 64, "y": 64, "z": 1 },
  "dim_blocks": { "x": 16, "y": 16, "z": 1 },
  "shared_mem_byte": 1024
}
```

### Return Data

```json
{}
```
