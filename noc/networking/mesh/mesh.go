// Package mesh provides a connector implementation for the mesh.
package mesh

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/sim/bottleneckanalysis"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/networking/networkconnector"
)

type tile struct {
	ports []sim.Port
	sw    int
	rt    *meshRoutingTable
}

// A Connector can help establishing a mesh or torus network.
type Connector struct {
	connector networkconnector.Connector

	freq                 sim.Freq
	switchLatency        int
	flitSize             int
	linkTransferPerCycle float64

	gridSize [3]int
	gridCap  [3]int
	grid     [][][]tile
	dstTable map[string]*tile
}

// NewConnector creates a new mesh Connector.
func NewConnector() *Connector {
	c := &Connector{
		freq:                 1 * sim.GHz,
		flitSize:             16,
		linkTransferPerCycle: 1,
		dstTable:             make(map[string]*tile),
	}

	c.connector = networkconnector.
		MakeConnector().
		WithFlitSize(c.flitSize)

	return c
}

// WithEngine sets the engine to be used.
func (c *Connector) WithEngine(e sim.Engine) *Connector {
	c.connector = c.connector.WithEngine(e)
	return c
}

// WithFreq sets the frequency that the network works at.
func (c *Connector) WithFreq(freq sim.Freq) *Connector {
	c.freq = freq
	c.connector = c.connector.WithDefaultFreq(freq)
	return c
}

// WithSwitchLatency sets the latency on each switch.
func (c *Connector) WithSwitchLatency(numCycles int) *Connector {
	c.switchLatency = numCycles
	return c
}

// WithBandwidth sets the bandwidth on each link in the unit of how many
// transfers per cycle.
func (c *Connector) WithBandwidth(transferPerCycle float64) *Connector {
	c.linkTransferPerCycle = transferPerCycle
	return c
}

// WithVisTracer sets the tracer used to trace tasks in the network.
func (c *Connector) WithVisTracer(t tracing.Tracer) *Connector {
	c.connector = c.connector.WithVisTracer(t)
	return c
}

// WithNoCTracer sets the tracer used to trace NoC-specific metrics, such as the
// traffics and congestions in the channels.
func (c *Connector) WithNoCTracer(t tracing.Tracer) *Connector {
	c.connector = c.connector.WithNoCTracer(t)
	return c
}

// WithFlitSize sets the flit size of the network.
func (c *Connector) WithFlitSize(size int) *Connector {
	c.flitSize = size
	c.connector = c.connector.WithFlitSize(size)
	return c
}

// WithMonitor sets a monitor that can inspect the internal states of the
// components in the network.
func (c *Connector) WithMonitor(monitor *monitoring.Monitor) *Connector {
	c.connector = c.connector.WithMonitor(monitor)
	return c
}

// WithBufferAnalyzer sets that buffer analyzer that can be used to record the
// buffer level in the mesh.
func (c *Connector) WithBufferAnalyzer(
	analyzer *bottleneckanalysis.BufferAnalyzer,
) *Connector {
	c.connector = c.connector.WithBufferAnalyzer(analyzer)
	return c
}

// CreateNetwork starts the process of creating a network. It also resets the
// connector if the connector has been used to create another network.
func (c *Connector) CreateNetwork(name string) {
	c.connector.NewNetwork(name)

	c.gridSize = [3]int{0, 0, 0}
	c.gridCap = [3]int{8, 8, 2}

	c.grid = c.initializeGrid(c.gridCap)
}

// AddTile adds some ports to a give coordinate in the mesh or torus. The
// connector supports 3 dimensional meshes. If a 2D mesh is designed, simply
// set the third coordinate to 0. We assume first first tile always has a
// coordinate of [0,0,0]. Negative coordinate is not supported.
func (c *Connector) AddTile(loc [3]int, ports []sim.Port) {
	if loc[0] < 0 || loc[1] < 0 || loc[2] < 0 {
		panic("coordinate is negative")
	}

	c.resizeGridToHold(loc)
	c.updateSize(loc)

	c.mergePorts(loc, ports)

	for _, port := range ports {
		c.dstTable[port.Name()] = &c.grid[loc[0]][loc[1]][loc[2]]
	}
}

func (c *Connector) mergePorts(loc [3]int, ports []sim.Port) {
	if len(c.grid[loc[0]][loc[1]][loc[2]].ports) != 0 {
		fmt.Printf("Tile [%d, %d, %d] already configured. Merging Ports.\n",
			loc[0], loc[1], loc[2])
	}

	c.grid[loc[0]][loc[1]][loc[2]].ports = append(
		c.grid[loc[0]][loc[1]][loc[2]].ports, ports...)
}

func (c *Connector) resizeGridToHold(loc [3]int) {
	resizeNeeded := false

	newGridCap := [3]int{c.gridCap[0], c.gridCap[1], c.gridCap[2]}
	if loc[0] >= c.gridCap[0] {
		newGridCap[0] = loc[0] * 2
		resizeNeeded = true
	}

	if loc[1] >= c.gridCap[1] {
		newGridCap[1] = loc[1] * 2
		resizeNeeded = true
	}

	if loc[2] >= c.gridCap[2] {
		newGridCap[2] = loc[2] * 2
		resizeNeeded = true
	}

	if !resizeNeeded {
		return
	}

	newGrid := c.initializeGrid(newGridCap)
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				newGrid[x][y][z] = c.grid[x][y][z]
			}
		}
	}

	c.grid = newGrid
	c.gridCap = newGridCap
}

func (c *Connector) updateSize(loc [3]int) {
	if loc[0] >= c.gridSize[0] {
		c.gridSize[0] = loc[0] + 1
	}

	if loc[1] >= c.gridSize[1] {
		c.gridSize[1] = loc[1] + 1
	}

	if loc[2] >= c.gridSize[2] {
		c.gridSize[2] = loc[2] + 1
	}
}

func (c *Connector) initializeGrid(cap [3]int) [][][]tile {
	grid := make([][][]tile, cap[0])
	for x := 0; x < cap[0]; x++ {
		grid[x] = make([][]tile, cap[1])
		for y := 0; y < cap[1]; y++ {
			grid[x][y] = make([]tile, cap[2])
			for z := 0; z < cap[2]; z++ {
				grid[x][y][z] = tile{
					rt: &meshRoutingTable{
						x: x,
						y: y,
						z: z,
					},
				}
			}
		}
	}

	return grid
}

// EstablishNetwork creates the switches, links, and the routing tables for the
// network to built.
func (c *Connector) EstablishNetwork() {
	c.createSwitches()
	c.createLinks()

	// router := &meshRouter{}
	// c.connector = c.connector.WithRouter(router)
	// c.connector.EstablishRoute()
}

func (c *Connector) createLinks() {
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				c.connectWithFrontSwitch(x, y, z)
				c.connectWithTopSwitch(x, y, z)
				c.connectWithLeftSwitch(x, y, z)
			}
		}
	}
}

func (c *Connector) createSwitches() {
	for x := 0; x < c.gridSize[0]; x++ {
		for y := 0; y < c.gridSize[1]; y++ {
			for z := 0; z < c.gridSize[2]; z++ {
				swName := fmt.Sprintf("SW[%d][%d][%d]", x, y, z)
				rt := &meshRoutingTable{
					x:        x,
					y:        y,
					z:        z,
					dstTable: c.dstTable,
				}
				sw := c.connector.AddSwitchWithNameAndRoutingTable(swName, rt)

				c.grid[x][y][z].rt = rt
				c.grid[x][y][z].sw = sw

				transferPerCycle := int(math.Ceil(c.linkTransferPerCycle))
				epName := fmt.Sprintf("EP[%d][%d][%d]", x, y, z)
				_, swPort := c.connector.ConnectDeviceWithEPName(
					epName,
					sw, c.grid[x][y][z].ports,
					networkconnector.DeviceToSwitchLinkParameter{
						DeviceEndParam: networkconnector.LinkEndDeviceParameter{
							IncomingBufSize:  transferPerCycle,
							OutgoingBufSize:  transferPerCycle,
							NumInputChannel:  transferPerCycle,
							NumOutputChannel: transferPerCycle,
						},
						SwitchEndParam: networkconnector.LinkEndSwitchParameter{
							IncomingBufSize:  transferPerCycle,
							OutgoingBufSize:  transferPerCycle,
							Latency:          1,
							NumInputChannel:  transferPerCycle,
							NumOutputChannel: transferPerCycle,
						},
						LinkParam: networkconnector.LinkParameter{
							IsIdeal:       true,
							Frequency:     c.freq,
							NumStage:      0,
							CyclePerStage: 0,
							PipelineWidth: 0,
						},
					})

				rt.local = swPort
			}
		}
	}
}

func (c *Connector) connectWithLeftSwitch(x, y, z int) {
	if x == 0 {
		return
	}

	x1 := x - 1

	curr := c.grid[x][y][z]
	left := c.grid[x1][y][z]

	portA, portB := c.createLink(left.sw, curr.sw, "Right", "Left")
	left.rt.right = portA
	curr.rt.left = portB
}

func (c *Connector) connectWithTopSwitch(x, y, z int) {
	if y == 0 {
		return
	}

	y1 := y - 1

	curr := c.grid[x][y][z]
	top := c.grid[x][y1][z]

	portA, portB := c.createLink(top.sw, curr.sw, "Bottom", "Top")
	top.rt.bottom = portA
	curr.rt.top = portB
}

func (c *Connector) connectWithFrontSwitch(x, y, z int) {
	if z == 0 {
		return
	}

	z1 := z - 1

	curr := c.grid[x][y][z]
	front := c.grid[x][y][z1]

	portA, portB := c.createLink(front.sw, curr.sw, "Back", "Front")
	front.rt.back = portA
	curr.rt.front = portB
}

func (c *Connector) createLink(
	a, b int,
	DirectionA, DirectionB string,
) (portA, portB sim.Port) {
	transferPerCycle := int(math.Ceil(c.linkTransferPerCycle))
	return c.connector.ConnectSwitches(a, b,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  transferPerCycle,
				OutgoingBufSize:  transferPerCycle,
				Latency:          c.switchLatency,
				NumInputChannel:  transferPerCycle,
				NumOutputChannel: transferPerCycle,
				PortName:         DirectionA,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  transferPerCycle,
				OutgoingBufSize:  transferPerCycle,
				Latency:          c.switchLatency,
				NumInputChannel:  transferPerCycle,
				NumOutputChannel: transferPerCycle,
				PortName:         DirectionB,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       false, // Use channel model for NoC tracing
				Frequency:     c.freq * sim.Freq(c.linkTransferPerCycle),
				NumStage:      1,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}
