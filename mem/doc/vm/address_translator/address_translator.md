# Address Translator

## Overview

Address Translator is a component that forwards memory read and write requests
with the translated addres Assuming that the core is using virtual addresses
and the L1 cache is using physical addresses, an address translator can be
placed in between the core and the L1 cache to perform the translation. The
purpose of the Address Translator component is to decouple the memory logic
with address translation and to enable users to configure the address
translation at any level.

## Parameters

## Ports

An Address Translator defines 3 ports: `TopPort`, `BottomPort`, and
`TranslationPort`.

## Protocol

Address translator receives `mem.ReadReq` and `mem.WriteReq` from the
`TopPort`, it guarantees that a corresponding type of response return to the
request source at some time in the future.

Address translator sends `vm.TranslationReq` to the Translation Service
Provider (TSP) through the `TranslationPort`. The TSP should guarantee the
address get translated and sent back from the `TranslationPort` in the future.

Address translator also send `mem.ReadReq` and `mem.WriteReq` to other memory
or cache component throught the `BottomPort`. Address translator expects to
receive the response to the read and write request form the `BottomPort`.

## Timing

Address Translator follows a pipelined design described in the following
diagram.

![Address Translator Pipeline](address_translator.png)

1. Read and write requests that arrive at the Address Translator from the
   `TopPort` are extraced by the `Translate` stage. The `Translate` stage
   generates a `vm.TranslationReq` to the TSP and sent through the
   `TranslationPort`. In case the same page is currently being translated, no
   new request will be generated.

2. `Forward` stage extracts the translation response from the
   `TranlsationPort`. It clones the original read and write requests and
   replaces the address with the translated physical address and replaces the
   PID with 0 (for physical address). The `Forward` stage sends the cloned
   requests throught the bottom port to the memory unit that can fulfill the
   read and the writ request. In case there are multiple requests for the same
   page, the forwarder forwards one request per cycle.

3. The `Respond` stage receives responses from the `BottomPort` and sends
   the responds to the original requester throught the `TopPort`.
