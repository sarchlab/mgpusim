package cfd

import (
	"slices"
)

type point [3]int
type tetrahedron [4]point
type face [3]point

type neighbor struct {
	offset      point
	tetrahedron int
}

type topology struct {
	tetrahedra [6]tetrahedron
	adjacent   [6][4]neighbor
	normals    [6][4][3]float32
}

func subtract(a, b point) point { return point{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func cross(a, b point) point {
	return point{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func dot(a, b point) int { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func tetraFace(t tetrahedron, opposite int, shift point) face {
	var plane face
	at := 0
	for i, p := range t {
		if i != opposite {
			plane[at] = point{p[0] + shift[0], p[1] + shift[1], p[2] + shift[2]}
			at++
		}
	}
	slices.SortFunc(plane[:], func(a, b point) int {
		for i := range a {
			if a[i] != b[i] {
				return a[i] - b[i]
			}
		}
		return 0
	})
	return plane
}

func faceNeighbor(tetrahedra [6]tetrahedron, original int, key face) neighbor {
	var result neighbor
	found := 0
	for z := -1; z <= 1; z++ {
		for y := -1; y <= 1; y++ {
			for x := -1; x <= 1; x++ {
				offset := point{x, y, z}
				for i, t := range tetrahedra {
					if offset == (point{}) && i == original {
						continue
					}
					for f := range t {
						if tetraFace(t, f, offset) == key {
							result = neighbor{offset, i}
							found++
						}
					}
				}
			}
		}
	}
	if found != 1 {
		panic("each tetrahedral face must have one neighbor")
	}
	return result
}

func meshTopology() topology {
	permutations := [6]point{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	var t topology
	for i, perm := range permutations {
		tet := tetrahedron{3: point{1, 1, 1}}
		tet[1][perm[0]] = 1
		tet[2] = tet[1]
		tet[2][perm[1]] = 1
		t.tetrahedra[i] = tet
	}
	for i, tet := range t.tetrahedra {
		for f := range tet {
			p := tetraFace(tet, f, point{})
			normal := cross(subtract(p[1], p[0]), subtract(p[2], p[0]))
			sign := float32(-0.5)
			if dot(normal, subtract(tet[f], p[0])) > 0 {
				sign = 0.5
			}
			for d, x := range normal {
				t.normals[i][f][d] = float32(x) * sign
			}
			t.adjacent[i][f] = faceNeighbor(t.tetrahedra, i, p)
		}
	}
	return t
}

func (b *Benchmark) meshInput() {
	n := b.Padded
	side := 1
	for 6*side*side*side < b.N {
		side++
	}
	b.Initial = make([]float32, 5*n)
	b.Normals = make([]float32, 12*n)
	b.Areas = make([]float32, n)
	b.Neighbors = make([]int32, 4*n)
	m := meshTopology()
	for i := 0; i < b.N; i++ {
		cube, kind := i/6, i%6
		x, y, z := cube%side, cube/side%side, cube/(side*side)
		for f, a := range m.adjacent[kind] {
			xx, yy, zz := x+a.offset[0], y+a.offset[1], z+a.offset[2]
			other := 6*((zz*side+yy)*side+xx) + a.tetrahedron
			outside := xx < 0 || xx >= side || yy < 0 || yy >= side || zz < 0 || zz >= side || other < 0 || other >= b.N
			if outside {
				other = -2
				p := tetraFace(m.tetrahedra[kind], f, point{})
				if y == 0 && p[0][1] == 0 && p[1][1] == 0 && p[2][1] == 0 {
					other = -1
				}
			}
			b.Neighbors[i+f*n] = int32(other)
			for d, c := range m.normals[kind][f] {
				b.Normals[i+(f+d*4)*n] = c
			}
		}
		perturb := float32((11*x+7*y+3*z+kind)%33-16) / 256
		density, pressure := float32(1.4)+perturb, 1+0.5*perturb
		const speed float32 = 1.2
		b.Initial[i], b.Initial[i+n] = density, density*speed
		b.Initial[i+4*n] = float32(density*(0.5*(speed*speed))) + pressure/gammaMinusOne
	}
	for i := range b.Areas {
		b.Areas[i] = 1.0 / 6.0
	}
	for i := b.N; i < n; i++ {
		for f := 0; f < 4; f++ {
			b.Neighbors[i+f*n] = b.Neighbors[b.N-1+f*n]
		}
		for f := 0; f < 12; f++ {
			b.Normals[i+f*n] = b.Normals[b.N-1+f*n]
		}
		for f := 0; f < 5; f++ {
			b.Initial[i+f*n] = b.Initial[b.N-1+f*n]
		}
	}
}
