package sim

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

// Middlewares returns the list of middleware.
func (holder *MiddlewareHolder) Middlewares() []Middleware {
	return holder.middlewares
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
