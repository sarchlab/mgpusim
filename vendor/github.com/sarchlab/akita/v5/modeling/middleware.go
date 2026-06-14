package modeling

// Middleware defines the actions of a component.
type Middleware interface {
	// Tick processes a tick event. It returns true if progress is made.
	Tick() bool
}

// MiddlewareHolder can maintain a list of middleware.
type MiddlewareHolder struct {
	middlewares []Middleware
}

// AddMiddleware adds a middleware to the holder.
func (holder *MiddlewareHolder) AddMiddleware(middleware Middleware) {
	holder.middlewares = append(holder.middlewares, middleware)
}

// Middlewares returns a copy of the middleware list. The copy prevents callers
// from mutating the holder's internal slice; the middleware objects themselves
// are shared.
func (holder *MiddlewareHolder) Middlewares() []Middleware {
	middlewares := make([]Middleware, len(holder.middlewares))
	copy(middlewares, holder.middlewares)

	return middlewares
}

// Tick processes a tick event. It returns true if progress is made.
func (holder *MiddlewareHolder) Tick() bool {
	progress := false

	for _, middleware := range holder.middlewares {
		if middleware.Tick() {
			progress = true
		}
	}

	return progress
}
