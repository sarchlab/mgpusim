package matrixmultiplication

// Matrix defines a matrix
type Matrix struct {
	Data          []float32
	Width, Height uint32
}

// NewMatrix returns a matrix
func NewMatrix(width, height uint32) *Matrix {
	matrix := new(Matrix)
	matrix.Width = width
	matrix.Height = height
	matrix.Data = make([]float32, width*height)
	return matrix
}
