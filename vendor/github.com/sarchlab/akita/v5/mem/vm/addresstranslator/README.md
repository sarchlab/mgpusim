# addresstranslator — Virtual-to-Physical Request Translator

Package `addresstranslator` provides an address-translating forwarder for the
Akita simulation framework. It sits in the virtual-memory subsystem between a
compute unit and the memory hierarchy: it intercepts `mem.ReadReq` and
`mem.WriteReq` messages carrying virtual addresses, obtains the page translation,
rewrites each request's address to the physical address, and forwards it
downstream.

## How It Works

The translator is driven by two middlewares.

### parseTranslateMW — accept and translate

For each incoming `mem.ReadReq` / `mem.WriteReq` on the `Top` port, the
translator computes the virtual page ID (`addr >> Log2PageSize << Log2PageSize`)
and sends a `vm.TranslationReq` on the `Translation` port to the provider
resolved by the `TranslationProviderMapper`. The original request's fields are
saved in a transaction record while the translation is outstanding. This
middleware also handles `mem.ControlReq` flush and reset commands on the
`Control` port.

### respondPipelineMW — forward and respond

1. **parseTranslation** — When a `vm.TranslationRsp` returns, the saved request is
   cloned with its address rewritten to `page.PAddr + offset` (where `offset` is
   the original address modulo the page size) and sent downstream on the `Bottom`
   port to the memory provider resolved by the `MemProviderMapper`.
2. **respond** — When a `mem.DataReadyRsp` / `mem.WriteDoneRsp` returns on
   `Bottom`, it is matched to the in-flight request and a corresponding response
   is sent back up on `Top` to the original requester.

Up to `NumReqPerCycle` translations and responses are handled each tick.

## Key Types

- `Spec` — immutable configuration: frequency, `Log2PageSize`, `DeviceID`, and
  `NumReqPerCycle`. Port buffer sizes are chosen by the caller when the port
  instances are built and assigned, not in the `Spec`.
- `State` — mutable runtime data: the in-flight translation transactions, the
  requests forwarded to the bottom port awaiting responses, and the flushing
  flag.
- `Resources` — external wiring; holds the `MemProviderMapper` and the
  `TranslationProviderMapper` (both `mem.AddressToPortMapper`).
- `Comp` — `modeling.Component[Spec, State, Resources]`.

## Builder Pattern

```go
spec := addresstranslator.DefaultSpec()
spec.DeviceID = 1

at := addresstranslator.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(addresstranslator.Resources{
        MemProviderMapper:         memMapper,
        TranslationProviderMapper: tlbMapper,
    }).
    Build("AddressTranslator")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{...})` | External wiring (memory and translation provider mappers) |

## Ports

`Build` declares the component's ports by logical name; it does not create the
port instances. After `Build`, the caller builds each port with
`modeling.MakePortBuilder` (choosing the buffer size) and attaches it with
`AssignPort`. The same `modeling.Registrar` passed to the builder is used to
build the ports. Every declared port must be assigned before the component is
ticked.

```go
at := addresstranslator.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(res).
    Build("AddressTranslator")

for _, name := range []string{"Top", "Bottom", "Translation", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(at).
        WithSpec(modeling.PortSpec{BufSize: 4}).
        Build(name)
    at.AssignPort(name, p)
}
```

- **Top**: accepts `mem.ReadReq` / `mem.WriteReq` (virtual addresses), returns
  `mem.DataReadyRsp` / `mem.WriteDoneRsp`.
- **Bottom**: forwards translated `mem.ReadReq` / `mem.WriteReq` (physical
  addresses), receives `mem.DataReadyRsp` / `mem.WriteDoneRsp`.
- **Translation**: sends `vm.TranslationReq`, receives `vm.TranslationRsp`.
- **Control**: accepts `mem.ControlReq` (flush / reset), returns `mem.ControlRsp`.
