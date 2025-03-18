package bfs

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type node struct {
	index     int32
	neighbors []*node
}

type graph struct {
	nodes []*node
}

func (g *graph) generate(numNode, degree int) {
	g.nodes = make([]*node, 0, numNode)
	for i := 0; i < numNode; i++ {
		g.nodes = append(g.nodes, &node{index: int32(i)})
	}

	curr := 0
	for i := 0; i < degree; i++ {
		curr++
		if curr >= numNode {
			break
		}

		g.nodes[i].neighbors = append(g.nodes[i].neighbors, g.nodes[curr])
	}
}

func (g *graph) loadGraph(path string) {
	graphFile, err := os.Open(path)
	if err != nil {
		log.Panic(err)
	}
	reader := bufio.NewReader(graphFile)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Panic(err)
			}
			break
		}
		validLine := strings.Split(line, "#")[0]

		if len(validLine) == 0 {
			break
		}

		var nodeFrom, nodeTo int32
		n, err := fmt.Sscanf(validLine, "%d %d", &nodeFrom, &nodeTo)
		if n != 2 {
			continue
		}
		if err != nil {
			log.Panic("cannot scan from " + path)
		}

		g.addEdge(nodeFrom, nodeTo)
	}

	graphFile.Close()

	g.dump()
}

func (g *graph) addEdge(from, to int32) {
	g.grow(from)
	g.grow(to)

	g.nodes[from].neighbors = append(
		g.nodes[from].neighbors, g.nodes[to])
}

func (g *graph) grow(index int32) {
	for int32(len(g.nodes))-1 < index {
		g.nodes = append(g.nodes, &node{index: int32(len(g.nodes))})
	}
}

func (g *graph) asEdgeList() (edgeOffset []int32, edgeList []int32) {
	offset := int32(0)

	for _, from := range g.nodes {
		edgeOffset = append(edgeOffset, offset)

		for _, to := range from.neighbors {
			edgeList = append(edgeList, to.index)
		}

		offset += int32(len(from.neighbors))
	}

	edgeOffset = append(edgeOffset, offset)

	return edgeOffset, edgeList
}

func (g *graph) dump() {
	fmt.Printf("***** GRAPH *****\n")

	for _, from := range g.nodes {
		fmt.Printf("%d: ", from.index)
		for _, to := range from.neighbors {
			fmt.Printf("%d ", to.index)
		}
		fmt.Printf("\n")
	}

	fmt.Printf("*****  END  *****\n")
}
