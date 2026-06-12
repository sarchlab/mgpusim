// Package modeling provides the application-level component framework
// for the Akita discrete-event simulation system.
//
// It builds on the sim package's core types (Component, TickingComponent,
// Engine, Port) to provide:
//
//   - Generic Spec+State pattern via Component[S, T]
//   - Middleware pipeline for tick-based components
//   - EventDrivenComponent[S, T] for event-driven components
//   - JSON-based save/load (checkpoint/restore)
//   - Validation helpers for Spec and State types
//
// The sim package provides the simulation kernel (engine, events, ports,
// hooks). The modeling package provides the higher-level abstractions that
// application developers typically use to build components.
package modeling
