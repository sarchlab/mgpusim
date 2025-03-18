package optimization

// An Alg can uses the gradient to update model parameters.
type Alg interface {
	UpdateParameters(l Layer)
}
