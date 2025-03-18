# L1V Cache

## Overview

L1V cache is the customized L1 cache for the GCN3 architecture.

## Ports

The L1V cache provides 3 ports, including `TopPort`, `BottomPort`,
`ControlPort`.

## Timing

### Coalesce Stage

The "Coalesce Stage" extracts requests from the `TopPort`. If this is the first
request or this request is accessing the same block as the previous requests,
the coalescer buffers the request. If the request cannot coalesce with buffered
requests, the Coalesce Stage send the all the requests in the buffer as a
bundle to the directory stage. The Coalesce Stage also clears the buffer and
put the new request in the buffer. In the case that the request is the last
request from a wave instruction, the Coalesce Stage immediately performs
coalescing.

### Directory Stage

The "Directory Stage" considers 6 cases:

1. Read MSHR hit:

    In this case, the read transaction is attached to the MSHR entry. Nothing
    else should happen.

1. Read hit:

    The read transaction will be sent to the bank.

1. Read miss:

    A read request that fetches the block data is sent through the BottomPort.

1. Write MSHR hit:

    The write transaction is attached to the MSHR entry. When the data for the MSHR returns, the Bottom Parser merges the write with the fetched data. A write request is also sent through the BottomPort.

1. Write hit:

    The write transaction will be sent to the bank. A write request is also sent throught the BottomPort.

1. Write miss:

   A victim block will be found. The write transaction will be sent to the bank to overwrite the victim block. If this write writes a full line, the block is marked as valid. A write request is also sent throught the BottomPort.

### Bank Stage

### Parse Bottom Stage

The Parse Bottom Stage extracts responds from the `BottomPort`.

For write done response, the Parse Bottom Stage attach the done respond to the
pre-coalesed transaction so that those transactions can be resonded to the top
unit.

For data ready response, the Parse Bottom stage attach the data ready respond
to all the pre-coalescing transaction related. At the same time, the Parse
Bottom Stage also sent the transaction to the bank to store the fetched data.
In case the MSHR entry contains write transaction, the Parse Bottom Stage
merges the write with the fetch the data and send to the bank.

### Respond Stage

The respond stage does not have any input buffer. It watchs the first
transaction in the pre-coalescing transaction queue. When the transaction is
completed, the respond stage sends back the response to the top. The global
transaction queue guarantees that the return order to be the same as receive
order.