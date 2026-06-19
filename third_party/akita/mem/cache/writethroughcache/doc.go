// Package writethroughcache provides a unified GPU cache implementation supporting
// multiple write policies (write-around, write-evict, write-through) via the
// Spec.WritePolicyType string field.
//
// Components are built with the minimal builder: configuration is supplied as a
// whole through WithSpec (start from DefaultSpec()), the engine and registration
// come from WithRegistrar, and shared/external wiring (storage, the
// address-to-port mapper, and remote ports) is injected through WithResources.
// Build declares the component's Top, Bottom, and Control ports; the port
// instances are built with modeling.MakePortBuilder and attached after Build
// with AssignPort, so the caller chooses the buffer sizes.
package writethroughcache
