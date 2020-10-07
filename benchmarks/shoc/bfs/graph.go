package bfs

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type graph struct {
	edgeOffsets    []int32
	edgeList       []int32
	nodesWithEdges []int32
	nodeToIDMap    map[int32]int32
	IDToNodeMap    map[int32]int32
	edgeListMap    map[int32][]int32
}

func (g *graph) generate(numNode, degree int) {
	g.edgeOffsets = make([]int32, numNode+1)
	g.edgeList = make([]int32, numNode*(degree+1))

	offset := int32(0)

	for i := 0; i < numNode; i++ {
		g.edgeOffsets[i] = offset
		for j := 0; j < degree; j++ {
			temp := int32(i*degree + (j + 1))
			if temp < int32(numNode) {
				g.edgeList[offset] = temp
				offset++
			}
		}
		// if i != 0 {
		// 	g.edgeList[offset] =
		// 		int32(math.Floor(float64(i-1) / float64(degree)))
		// 	offset++
		// }
	}
	g.edgeOffsets[numNode] = offset
}

func (g *graph) generateFromText(path string) (int, int, int) {
	g.loadGraph(path)

	numNode := len(g.nodeToIDMap)
	g.nodesWithEdges = make([]int32, numNode+1)
	g.edgeOffsets = make([]int32, numNode+1)
	g.edgeList = make([]int32, 0)

	offset := int32(0)
	i := int32(0)
	edgeCount := 0
	for _, id := range g.nodeToIDMap {
		g.nodesWithEdges[i] = id
		if len(g.edgeListMap[id]) == 0 {
			if offset == 0 {
				g.edgeOffsets[i] = 0
			} else {
				g.edgeOffsets[i] = offset - 1
			}
		} else {
			g.edgeOffsets[i] = offset
		}

		edgeCount = edgeCount + len(g.edgeListMap[id])
		for _, val := range g.edgeListMap[id] {
			g.edgeList = append(g.edgeList, val)
			offset++
		}
		i++
	}

	degree := edgeCount / numNode
	return numNode, edgeCount, degree
}

func (g *graph) loadGraph(path string) {
	g.nodeToIDMap = make(map[int32]int32)
	g.IDToNodeMap = make(map[int32]int32)
	g.edgeListMap = make(map[int32][]int32)
	currNodeID := int32(0)

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

		if len(validLine) > 0 {
			var nodeFrom, nodeTo int32
			n, err := fmt.Sscanf(validLine, "%d %d", &nodeFrom, &nodeTo)
			if n != 2 {
				continue
			}
			if err != nil {
				log.Panic("cannot scan from " + path)
			}

			if _, ok := g.nodeToIDMap[nodeFrom]; !ok {
				g.nodeToIDMap[nodeFrom] = currNodeID
				g.IDToNodeMap[currNodeID] = nodeFrom
				currNodeID = currNodeID + 1
			}

			if _, ok := g.nodeToIDMap[nodeTo]; !ok {
				g.nodeToIDMap[nodeTo] = currNodeID
				g.IDToNodeMap[currNodeID] = nodeTo
				currNodeID = currNodeID + 1
			}

			nodeFromID := g.nodeToIDMap[nodeFrom]
			nodeToID := g.nodeToIDMap[nodeTo]
			g.edgeListMap[nodeFromID] = append(g.edgeListMap[nodeFromID], nodeToID)
		}
	}
	graphFile.Close()
}

func (g graph) Dump(mode string) {
	fmt.Printf("***** GRAPH *****")
	if mode == "auto" {
		for i := 0; i < len(g.edgeOffsets)-1; i++ {
			fmt.Printf("\nNode %d: ", i)
			for j := g.edgeOffsets[i]; j < g.edgeOffsets[i+1]; j++ {
				fmt.Printf("%d, ", g.edgeList[j])
			}
		}
	} else {
		for key, ele := range g.edgeListMap {
			fmt.Printf("\nNode %d: ", g.IDToNodeMap[key])
			for _, val := range ele {
				fmt.Printf("%d ", g.IDToNodeMap[val])
			}
		}
	}
	fmt.Printf("\n*****  END  *****\n")
}
