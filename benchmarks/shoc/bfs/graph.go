package bfs

import (
	"fmt"
)

type graph struct {
	edgeOffsets []int32
	edgeList    []int32
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

func (g graph) Dump() {
	fmt.Printf("***** GRAPH *****")
	for i := 0; i < len(g.edgeOffsets)-1; i++ {
		fmt.Printf("\nNode %d: ", i)
		for j := g.edgeOffsets[i]; j < g.edgeOffsets[i+1]; j++ {
			fmt.Printf("%d, ", g.edgeList[j])
		}
	}
	fmt.Printf("\n*****  END  *****\n")
}
