package messaging

import (
	"fmt"
	"reflect"
	"sync"
)

// A Protocol is a named, immutable set of message types that travel over a
// port, organized into roles. Defining a protocol with DefineProtocol
// registers every message type it carries with the checkpoint codec, so a
// message type that belongs to a protocol can always be decoded when a
// checkpoint captures it in a port buffer.
type Protocol struct {
	name  string
	roles []*Role
}

// Name returns the protocol's name.
func (p *Protocol) Name() string {
	return p.name
}

// Role returns the role with the given name. It panics if the protocol does
// not define the role.
func (p *Protocol) Role(name string) *Role {
	for _, r := range p.roles {
		if r.name == name {
			return r
		}
	}

	panic(fmt.Sprintf(
		"protocol %q does not define role %q", p.name, name))
}

// Messages returns the union of all roles' sends: every message type the
// protocol carries.
func (p *Protocol) Messages() []Msg {
	msgs := make([]Msg, 0, len(p.roles)*2)
	for _, r := range p.roles {
		msgs = append(msgs, r.sends...)
	}

	return msgs
}

// A Role is one endpoint's view of a protocol: the messages it sends. What a
// role receives is whatever the protocol's other roles send. Ports declare the
// role(s) they speak with DeclarePort.
type Role struct {
	protocol *Protocol
	name     string
	sends    []Msg
}

// Protocol returns the protocol this role belongs to.
func (r *Role) Protocol() *Protocol {
	return r.protocol
}

// Name returns the role's name.
func (r *Role) Name() string {
	return r.name
}

// Sends returns the messages this role sends.
func (r *Role) Sends() []Msg {
	sends := make([]Msg, len(r.sends))
	copy(sends, r.sends)

	return sends
}

// RoleDef declares a role and the messages it sends, as input to
// DefineProtocol.
type RoleDef struct {
	Name  string
	Sends []Msg
}

// protocolNames tracks every defined protocol name so a duplicate definition
// fails loudly at init time.
var (
	protocolNamesMu sync.Mutex
	protocolNames   = map[string]bool{}
)

// DefineProtocol creates a protocol and registers every message type across
// all roles with the checkpoint codec. Call it as a package-level var in the
// package that defines the message types:
//
//	var (
//	    Protocol  = messaging.DefineProtocol("mem",
//	        messaging.RoleDef{Name: "requester",
//	            Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
//	        messaging.RoleDef{Name: "responder",
//	            Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
//	    )
//	    Requester = Protocol.Role("requester")
//	    Responder = Protocol.Role("responder")
//	)
//
// It panics on a duplicate protocol name, a duplicate role name, or a message
// type listed in more than one role of the same protocol. A message type may
// belong to more than one protocol; re-registration with the codec is
// harmless.
func DefineProtocol(name string, roles ...RoleDef) *Protocol {
	if name == "" {
		panic("protocol name must not be empty")
	}

	if len(roles) == 0 {
		panic(fmt.Sprintf("protocol %q must define at least one role", name))
	}

	protocolNamesMu.Lock()
	if protocolNames[name] {
		protocolNamesMu.Unlock()
		panic(fmt.Sprintf("protocol %q is already defined", name))
	}
	protocolNames[name] = true
	protocolNamesMu.Unlock()

	p := &Protocol{name: name}
	seenRoles := map[string]bool{}
	seenMsgTypes := map[reflect.Type]string{}

	for _, def := range roles {
		if def.Name == "" {
			panic(fmt.Sprintf(
				"protocol %q: role name must not be empty", name))
		}

		if seenRoles[def.Name] {
			panic(fmt.Sprintf(
				"protocol %q: role %q is already defined", name, def.Name))
		}
		seenRoles[def.Name] = true

		for _, msg := range def.Sends {
			t := reflect.TypeOf(msg)
			if otherRole, seen := seenMsgTypes[t]; seen {
				panic(fmt.Sprintf(
					"protocol %q: message type %s is sent by both role %q "+
						"and role %q; every message is sent by exactly one role",
					name, t, otherRole, def.Name))
			}
			seenMsgTypes[t] = def.Name

			msgCodec.Register(msg)
		}

		sends := make([]Msg, len(def.Sends))
		copy(sends, def.Sends)

		p.roles = append(p.roles, &Role{
			protocol: p,
			name:     def.Name,
			sends:    sends,
		})
	}

	return p
}
