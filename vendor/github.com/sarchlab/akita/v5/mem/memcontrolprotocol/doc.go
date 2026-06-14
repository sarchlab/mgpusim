// Package memcontrolprotocol is the protocol package for the uniform control protocol
// every memory agent in Akita speaks over its "Control" port. It defines the
// protocol itself (Protocol, with Requester/Responder roles), its messages
// (Req, Rsp) and verbs (Command, the Cmd* constants), the shared control
// state model, and a reusable conformance harness:
//
//   - State, the shared enumeration of where a memory agent sits in its
//     control lifecycle. Components hold a value of this type in their
//     own state structs so the bookkeeping is uniform and serializable.
//
//   - VerbSupport, a per-component declaration of which verbs the
//     component implements. Verbs that are not declared supported must
//     respond with Rsp{Success: false, Error: "unsupported"}.
//
//   - RunContract, a *testing.T-based harness that exercises every verb
//     against a built component over its real Control port. Each
//     component package adds one test that calls RunContract with its
//     build function and its declared VerbSupport.
//
// See mem/CONTROL_PROTOCOL.md for the verb definitions, response timing,
// support matrix, and per-component behavior.
package memcontrolprotocol
