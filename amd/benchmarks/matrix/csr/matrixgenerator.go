package csr

import "math/rand"

// MatrixGenerator defines a matrix generator
type MatrixGenerator struct {
	numNode, numConnection   uint32
	xCoords, yCoords         []uint32
	values                   []float32
	positionOccupied         map[uint32]bool
	xCoordIndex, yCoordIndex map[uint32][]uint32
}

// MakeMatrixGenerator returns a matrixGenerator
func MakeMatrixGenerator(numNode, numConnection uint32) MatrixGenerator {
	return MatrixGenerator{
		numNode:       numNode,
		numConnection: numConnection,
	}
}

// GenerateMatrix generates matrix
func (g MatrixGenerator) GenerateMatrix() Matrix {
	g.init()
	g.generateConnections()
	g.normalize()
	m := g.outputCSRFormat()
	return m
}

func (g *MatrixGenerator) init() {
	g.xCoords = make([]uint32, 0, g.numConnection)
	g.yCoords = make([]uint32, 0, g.numConnection)
	g.values = make([]float32, 0, g.numConnection)
	g.positionOccupied = make(map[uint32]bool)
	g.xCoordIndex = make(map[uint32][]uint32)
	g.yCoordIndex = make(map[uint32][]uint32)
}

func (g *MatrixGenerator) generateConnections() {
	for i := uint32(0); i < g.numConnection; i++ {
		g.generateOneConnection()
	}
}

func (g *MatrixGenerator) normalize() {
	for i := uint32(0); i < g.numNode; i++ {
		sum := g.sumColumn(i)
		if sum == 0 {
			continue
		}

		indexes := g.xCoordIndex[i]
		for _, index := range indexes {
			g.values[index] /= sum
		}
	}
}

func (g MatrixGenerator) outputCSRFormat() Matrix {
	m := Matrix{}
	rowOffset := uint32(0)

	for i := uint32(0); i < g.numNode; i++ {
		cols, values := g.selectRowData(i)
		g.sortRowData(cols, values)

		m.RowOffsets = append(m.RowOffsets, rowOffset)
		m.ColumnNumbers = append(m.ColumnNumbers, cols...)
		m.Values = append(m.Values, values...)
		rowOffset += uint32(len(cols))
	}
	m.RowOffsets = append(m.RowOffsets, rowOffset)

	return m
}

func (g MatrixGenerator) selectRowData(
	row uint32,
) (
	cols []uint32,
	values []float32,
) {
	indexes := g.yCoordIndex[row]
	for _, index := range indexes {
		cols = append(cols, g.xCoords[index])
		values = append(values, g.values[index])
	}
	return
}

func (g MatrixGenerator) sortRowData(cols []uint32, values []float32) {
	for i := 0; i < len(cols); i++ {
		for j := i; j < len(cols); j++ {
			if cols[i] >= cols[j] {
				cols[i], cols[j] = cols[j], cols[i]
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func (g MatrixGenerator) sumColumn(i uint32) float32 {
	sum := float32(0)
	indexes := g.xCoordIndex[i]
	for _, index := range indexes {
		sum += g.values[index]
	}
	return sum
}

func (g *MatrixGenerator) generateOneConnection() {
	x, y := g.generateUnoccupiedPosition()
	v := rand.Float32()
	g.xCoords = append(g.xCoords, x)
	g.yCoords = append(g.yCoords, y)
	g.values = append(g.values, v)

	if _, ok := g.xCoordIndex[x]; !ok {
		g.xCoordIndex[x] = make([]uint32, 0)
	}
	if _, ok := g.yCoordIndex[y]; !ok {
		g.yCoordIndex[y] = make([]uint32, 0)
	}
	g.xCoordIndex[x] = append(g.xCoordIndex[x], uint32(len(g.values)-1))
	g.yCoordIndex[y] = append(g.yCoordIndex[y], uint32(len(g.values)-1))
}

func (g MatrixGenerator) generateUnoccupiedPosition() (x, y uint32) {
	for {
		x = uint32(rand.Int()) % g.numNode
		y = uint32(rand.Int()) % g.numNode
		if !g.isPositionOccupied(x, y) {
			g.markPositionOccupied(x, y)
			return
		}
	}
}

func (g MatrixGenerator) isPositionOccupied(x, y uint32) bool {
	_, ok := g.positionOccupied[y*g.numNode+x]
	return ok
}

func (g MatrixGenerator) markPositionOccupied(x, y uint32) {
	g.positionOccupied[y*g.numNode+x] = true
}
