package modeling

// None is the resource type for components that reference no shared resources.
// It is a zero-size sentinel used as the third type argument of Component:
// Component[Spec, State, None].
type None struct{}

// Component is a generic component that combines Spec, State, Resources, Ports,
// and Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
// R is the Resources type (references to shared resources; None when unused).
//
// Component embeds [TickingComponent] for tick-based lifecycle management
// and [MiddlewareHolder] for the middleware pipeline.
//
// Spec and Resources are set once at construction and exposed only through
// read-only accessors, so they cannot be mutated after the builder establishes
// them. State stays a public field: middleware mutates it in place every tick,
// which requires cross-package write access that Go can only grant through an
// exported field.
type Component[S any, T any, R any] struct {
	*TickingComponent
	MiddlewareHolder

	spec      S
	State     T
	resources R
}

// Spec returns the component's immutable configuration. The returned value is a
// copy, so callers cannot mutate the builder-established configuration.
func (c *Component[S, T, R]) Spec() S {
	return c.spec
}

// Resources returns the component's shared-resource references. The references
// are fixed at construction; only the state they point to mutates.
func (c *Component[S, T, R]) Resources() R {
	return c.resources
}

// Tick runs the middleware pipeline.
func (c *Component[S, T, R]) Tick() bool {
	return c.MiddlewareHolder.Tick()
}
