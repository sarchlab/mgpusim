// Package simulation provides the top-level simulation runner for the Akita
// simulation framework.
//
// A Simulation wires together the engine, data recorder, visual tracer, and
// optional monitoring server. It also acts as a global state manager: every
// registered runtime object — component, port, connection, shared-state
// resource, plus the engine and ID generator — is recorded in one flat entity
// inventory with a globally unique name. Serializing every entity's state
// therefore captures the complete state required to recover the simulation.
package simulation
