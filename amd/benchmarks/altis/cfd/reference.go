package cfd

import "math"

type conserved [5]float64
type physicalState struct {
	pressure, speed, sound float64
	flux                   [5][3]float64
}

func physical(u conserved) physicalState {
	var p physicalState
	v := [3]float64{u[1] / u[0], u[2] / u[0], u[3] / u[0]}
	squared := v[0]*v[0] + v[1]*v[1] + v[2]*v[2]
	p.pressure = float64(gammaMinusOne) * (u[4] - 0.5*u[0]*squared)
	p.speed = math.Sqrt(squared)
	p.sound = math.Sqrt(float64(float32(1.4)) * p.pressure / u[0])
	for d, value := range v {
		p.flux[0][d] = u[1+d]
		p.flux[4][d] = (u[4] + p.pressure) * value
		for m := 0; m < 3; m++ {
			p.flux[1+m][d] = value * u[1+m]
			if m == d {
				p.flux[1+m][d] += p.pressure
			}
		}
	}
	return p
}

func farPhysical(f conserved) physicalState {
	p := physical(f)
	for d := 0; d < 3; d++ {
		var velocity float32
		if d == 0 {
			velocity = 1.2
		}
		for m := 0; m < 3; m++ {
			value := float32(velocity * float32(f[m+1]))
			if d == m {
				value++
			}
			p.flux[m+1][d] = float64(value)
		}
		p.flux[4][d] = float64(velocity * float32(f[4]+1))
	}
	return p
}

func readCell(field []float64, n, cell int) conserved {
	var state conserved
	for i := range state {
		state[i] = field[cell+i*n]
	}
	return state
}

func (b *Benchmark) cellFlux(cell int, state []float64, farState physicalState) conserved {
	n := b.Padded
	u := readCell(state, n, cell)
	a := physical(u)
	var sum conserved
	for f := 0; f < 4; f++ {
		id := b.Neighbors[cell+f*n]
		normal := [3]float64{
			float64(b.Normals[cell+f*n]),
			float64(b.Normals[cell+(f+4)*n]),
			float64(b.Normals[cell+(f+8)*n]),
		}
		if id == -1 {
			for d, x := range normal {
				sum[1+d] += x * a.pressure
			}
			continue
		}
		other := farState
		if id >= 0 {
			v := readCell(state, n, int(id))
			other = physical(v)
			length := math.Sqrt(normal[0]*normal[0] + normal[1]*normal[1] + normal[2]*normal[2])
			viscosity := -length * float64(float32(0.2)) * 0.5 * (a.speed + other.speed + a.sound + other.sound)
			for k := range sum {
				sum[k] += viscosity * (u[k] - v[k])
			}
		}
		for d, x := range normal {
			for k := range sum {
				sum[k] += 0.5 * x * (a.flux[k][d] + other.flux[k][d])
			}
		}
	}
	return sum
}

func (b *Benchmark) reference() [4][]float64 {
	n := b.Padded
	state, old, flux, factor := make([]float64, 5*n), make([]float64, 5*n), make([]float64, 5*n), make([]float64, n)
	for i, v := range b.Initial {
		state[i] = float64(v)
	}
	var far conserved
	for i, v := range farfield() {
		far[i] = float64(v)
	}
	farState := farPhysical(far)
	for t := 0; t < timesteps; t++ {
		copy(old, state)
		for cell := 0; cell < n; cell++ {
			p := physical(readCell(state, n, cell))
			factor[cell] = 0.5 / (math.Sqrt(float64(b.Areas[cell])) * (p.speed + p.sound))
		}
		for stage := 0; stage < rkStages; stage++ {
			for cell := 0; cell < n; cell++ {
				sum := b.cellFlux(cell, state, farState)
				for field, force := range sum {
					flux[cell+field*n] = force
				}
			}
			for cell := 0; cell < n; cell++ {
				scale := factor[cell] / float64(4-stage)
				for field := 0; field < 5; field++ {
					i := cell + field*n
					state[i] = old[i] + scale*flux[i]
				}
			}
		}
	}
	return [4][]float64{state, old, flux, factor}
}
