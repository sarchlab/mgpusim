// Package messaging provides messages, ports, connections, and protocols.
//
// A protocol is a named set of message types organized into roles. Packages
// that define message types declare their protocol once with DefineProtocol,
// which registers every message type with the checkpoint codec, and ports
// declare the role(s) they speak in DeclarePort.
package messaging
