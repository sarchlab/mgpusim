// Package monitoring2 provides a Monitor type that wraps the Daisen2 server to
// offer simulation monitoring capabilities. It serves as the AkitaRTM
// monitoring library, allowing simulations to expose live monitoring
// endpoints for inspection of component states, progress bars, and
// visualization traces.
//
// Usage:
//
//	monitor := monitoring2.NewMonitor().WithPortNumber(8080)
//	monitor.RegisterEngine(engine)
//	monitor.RegisterComponent(component)
//	monitor.RegisterVisTracer(tracer)
//	monitor.StartServer()
package monitoring2
