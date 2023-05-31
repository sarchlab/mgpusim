# Writeback Cache

A writeback cache is a cache module that uses the writeback policy. 

A writeback cache has 3 ports, including the topPort, the bottomPort, and the controlPort. The Top Port receives a read and write requests and replies data-ready and write-done responses. The bottomPort sends read and write requests to another cache or memory component and expects data-ready and write-done responses. The control port handles special controlling requests, such as pause, continue, and flush.

![Write Back Cache Stage](writeback.png)

Here is a graph that illustrates the internal sub-components of a writeback cache. Rather than introducing each component, we introduce the component by describing the life cycles of different types of cache transactions and their journeys through all the components.

## Cache Transactions

All the cache transactions start from requests arriving at the top port. The Top Parser extracts requests from the Top Port buffer and wraps the requests as a transaction. One request produces one cache transaction. The Top Parser forwards all the transactions to the Directory Stage. The directory stages compare the request address with the current MSHR status and the data that is stored in the directory to determine what to do next. Here, we define the directory as the tags stored in the cache, rather than the directory for cache coherency protocols.

So the common first and second step for all the cache transactions are:

* Step 1: The **Top Parser** parses the request from a higher-level module.

* Step 2: The **Directory Stage** checks the MSHR state and the meta-data in the directory to determine the following actions.

The directory maintains the read and write reference count of each cache line. When the read reference is greater than 1, other reads can proceed, but writes have to wait. If the write reference is greater than 1, all reads and writes have to wait.

### Read, MSHR Hit

* Step 2: The **Directory Stage** attaches the transaction to the MSHR entry. 

* Step 3: The **MSHR Stage** prepares the response when the data is ready. The MSHR Stage sends the response through the Top Port.

### Read, MSHR Miss, Directory Hit

* Step 2: The **Directory Stage** add the cache line read reference count by 1 and sends the transaction to the bank stage.

* Step 3: The **Bank Stage** read from its local storage. When the local read is completed, the bank reduces the read reference of the cache line by 1. The bank also sends the response through the Top Port.

### Read, MSHR Miss, Directory Miss, No Eviction

In the case of a read miss, the writeback cache needs to read from a lower-level module. The directory needs to find a cache line to hold the data. The cache line is called a victim. When the cache line does not hold any dirty data, the data in that cache line can be safely removed without writing back to a lower-level model.

* Step 2: The **Directory Stage** add write reference count by 1 to the cache line. Since the bank needs to "write" the data to the cache line later, the directory adds the write reference rather than read reference. The directory stage also creates an MSHR entry. Eventually, the directory stage sends the request to the write buffer to fetch the cache line.

* Step 3: The **Write Buffer** checks if the cacheline is currently in the buffer. If not, send a read request to a lower-level module to fetch the data.

* Step 4: The **Write Buffer** collects the data for the fetch. The data can either reside in the write buffer. Otherwise, the write buffer waits for the return request sent to the lower-level module to return. When the write buffer has the data, the write buffer combines writes with the fetched data. The write buffer sends the data to the bank to write to the local storage. The corresponding MSHR entry is removed at this moment. 

    The MSHR entry cannot be removed at a later cycle, since the write-combining take place in this cycle. If the MSHR entry is still there, a new write may attach to the MSHR entry, and no logic can combine the write with the fetched data.

* Step 5: The **Bank Stage** writes the data locally. When complete, the Bank Stage reduces the write reference count by 1. The Bank Stage then sends the transaction to the MSHR stage.

* Step 6: The **MSHR Stage** prepares the responses for each request associated with the MSHR entry. The MSHR Stage sends the responses through the Top Port. 

### Read, MSHR Miss, Directory Miss, Need Eviction

In this case, the victim cacheline has dirty data and needs to write to the lower-level module.

* Step 2: The **Directory Stage** adds the write reference of the cache line by 1. Since fetch is required, the Directory Stage also creates an MSHR entry. Then the Directory Stage sends the transaction to the Bank Stage. 

* Step 3: The **Bank Stage** reads the data to be evicted and sends the transaction to the write buffer.

* Step 4: The **Write Buffer** adds the evicted data to the write buffer, and the write request will be issued to the lower-level module at a later time. 

* Step 5: The **Write Buffer** fetches the reading data, either from the local write buffer or from a lower-level module. Once the data is ready, the Write Buffer combines the fetched data with the write requests associated with the MSHR entry. The Write Buffer also removes the MSHR entry. Finally, the Write Buffer sends the transaction to the bank (for the 2nd time). 

* Step 6: The **Bank Stage** writes the fetched data to local storage and sends the transaction to the MSHR stage.

* Step 7: The **MSHR Stage** prepares the responses for each request associated with the MSHR entry.

### Write, MSHR Hit

* Step 2: The **Directory Stage** attaches the transaction with the MSHR entry.

* Step 3: The **Write Buffer**, once collected the data for the MSHR entry, combines the write with the collected data.

* Step 4: The **Bank Stage** writes the Write Buffer collected data to the local storage.

* Step 5: The **MSHR Stage** prepares the responses and sends the responses through the Top Port.

### Write, MSHR Miss, Directory Hit

This is the typical write hit case.

* Step 2: The **Directory Stage** adds the write reference by 1 and sends the transaction to the Bank Stage.

* Step 3: The **Bank Stage** writes the data locally, reduces the write reference by 1, and sends the response through the Top Port.

### Write, MSHR Miss, Directory Miss, No Eviction, Full Cacheline

This case is generally considered as "write miss." However, since it does not need to fetch data from a lower-level module, it is equivalent to a "write hit."

* Step 2: The **Directory Stage** adds the write reference by 1. It sends the transaction to the bank as if it is a write hit.

* Step 3: The **Bank Stage** writes the data, reduces the write reference by 1, and sends the response through the Top Port.

### Write, MSHR Miss, Directory Miss, Need Eviction, Full Cacheline

* Step 2: The **Directory Stage** adds the write reference by 1. It sends the transaction to the bank.

* Step 3: The **Bank Stage** reads the data for eviction and sends the transaction to the write buffer.

* Step 4: The **Write Buffer** buffers the evicted data. And send the transaction back to the bank as a write hit.

* Step 5: The **Bank Stage** writes the data to the local storage, reduces the write reference by 1, and sends the response through the Top Port.

### Write, MSHR Miss, Directory Miss, No Eviction, Partial Cacheline

* Step 2: The **Directory Stage** adds the write reference by 1. Since fetch is necessary, the directory stage creates an MSHR entry. The write is attached to the MSHR entry. The Directory Stage sends the transaction to the write buffer.

* Step 3: The **Write Buffer** collects the data, either from local write buffer or from a lower-level module. The write is combined with the fetched data. 

* Step 4: The **Bank Stage** writes the data to local storage. It also reduces the write reference by 1. 

* Step 5: The **MSHR Stage** generates the response and sends it through the Top Port.

## Write, MSHR Miss, Directory Miss, Need Eviction, Partial Cacheline.

* Step 2: The **Directory Stage** adds the write reference by 1. It also creates an MSHR entry for the write. The transaction is then sent to the bank.

* Step 3: The **Bank Stage** reads the victim data.

* Step 4: The **Write Buffer** sends the buffers the eviction.

* Step 5: The **Write Buffer** collects the data, either from local write buffer or from a lower-level module. The write is combined with the fetched data. 

* Step 6: The **Bank Stage** writes the data to local storage. It also reduces the write reference by 1. 

* Step 7: The **MSHR Stage** generates the response and send it through the Top Port.



## Control Requests

### Flushing

The writeback cache handles flush requests. In general, there are 4 steps to
flush a writeback cache. 

* Step 1 **Receive request**: The writeback cache receives the flush request
  from the Control Port. 

    If the flush request sets the bit "Discard Inflight" transactions, the
    writeback cache directly cancels all the on-going transactions, remove MSHR
    entries, release all the counters in cachelines. Also, all the messages that
    are currently in the Top Port is discarded.

* Step 2 **Pre-Flush**: The writeback cache waits until all the inflight
  transaction is completed. If the flush request sets the "Discard Inflight"
  bit, this stage will only consume a single cycle as all the inflight
  transactions are already discarded.

* Step 3 **Flush**: The writeback cache generate Evict transactions (1
  transaction for each dirty block) to banks. The banks will read the data and
  send the data to the write buffer to write to lower-level module. This step
  finishes when all the dirty blocks are evicted and when the write buffer is
  empty. 

 * Step 4 **Resume**: If the flush request sets the "Pause after Flushing" bit,
   the cache will be in a paused state after the flushing is completed. The
   cache waits for the Restart request before it can processing any new
   requests.  If the "Pause after Flushing" bit is not set, the cache will
   immediately start to process new requests after flushing.

## Write Buffer

The writeback cache implements a write buffer. The write buffer holds the data to be flushed to the lower-level cache.

### Transactions

* **Write Buffer Fetch** attempts to read data from the write buffer. The write
  buffer will first check within the buffer to see if it has the requested data.
  If the write buffer has the data, it combines the data in the MSHR entry and
  respond to the bank. Otherwise, the write buffer send request to the
  lower-level module to fetch the data. When data-ready is received, the write
  buffer merges the returned data into the MSHR entry and respond to the bank.

* **Write Buffer Flush** attempts to write data from the cache to a lower-level
  module. It will add an entry into pending evictions so that the data will
  later be written to a lower-level module.

* **Write Buffer Fetch and Evict** handles the case where a read access to the
  writeback cache needs to evict a dirty cacheline to make space. It is first
  treated as a **Write Buffer Flush** transaction for eviction. After writing
  the entry into the pending eviction list. Then, it is treated like a regular
  **Write Buffer Fetch** transaction

* **Write Buffer Evict and Write** handles the case where a write to the write
  back cache evicts a victim cacheline. If the write buffer is not full, the
  write buffer will buffer the eviction. Also, it respond to the cache bank
  immediately so that the bank perform the write operation. The real eviction
  write to the lower-level module can happen much later than the time when the
  bank writes the new cacheline.








