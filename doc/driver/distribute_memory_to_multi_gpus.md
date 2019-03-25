# Distribute Memory To Multiple GPUs

The Driver provides a convinent function that can distribute a section of
memory to multiple GPUs. The function is called `Distribute`. The signature of
the function is as follows:

```go
func (d *Driver) Distribute(
    c *Context,
    ptr, byteSize uint64,
    gpuIDs []int,
) (
    byteAllocatedOnEachGPU []uint64,
) {
    ...
}
```

This function takes the driver execution context as the first argument. It also takes the memory section to distribute, represented by the `ptr` and the `byteSize` argument. The `ptr` is an page-aligned address. Therfore, if the user wants to distribute the memory, the user has to allocate the memory with alignment. It also takes an array of integers as argument for the GPUs that the memory should be distributed to.

This function calls the `Remap` API provided by the driver. The `Remap` function modifies the page table to reassign the page to GPU mapping. Since the `Remap` function does not move the data. The user needs to call `Distribute` or `Remap` before memory copy. Ideally, `Distribute` and `Remap` should be called immediately after allocation.

The `Distribute` function does not guarantee event splitting. It will always assign whole pages to GPUs. Also, if the number of pages is not a multiple of the number of GPUs, the last GPU will have more pages. For example, if the user wants to distribute 7 pages to 3 GPUs (GPU 1, 2, 3), GPU 1 and 2 will get 2 pages while GPU 3 will get 3 pages. In addition, if the number of bytes to distrubute is not a multiple of the page size, the last GPU that is allocated a page will be get the extra bytes. Actually, since all the remapping happens at a page level, the whole page that contains the extra bytes will also assign to the that GPU. For example, support a user want to distribute 0x1000 to 0x2500 to 3 GPUs (GPU 1, 2, and 3). The result would be that GPU 1 has address 0x1000 - 0x1FFF, GPU 2 has address 0x2000 - 0x3FFF.
