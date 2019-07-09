package floydwarshall

import (
	"fmt"
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type FloydWarshallKernelArgs struct {
	OutputPathMatrix				driver.GPUPtr
	OutputPathDistanceMatrix        driver.GPUPtr

	numNodes  	uint32
	pass 		uint32
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.HsaCo

	Width              int
	elemsPerThread1Dim int
	blockSize          int

	hNumNodes						uint32
	hOutputPathMatrix				[]uint32
	hOutputPathDistanceMatrix		[]uint32
	dNumNodes					driver.GPUPtr
	dOutputPathMatrix			driver.GPUPtr
	dOutputPathDistanceMatrix	driver.GPUPtr

	hVerificationPathMatrix				[]uint32
	hVerificationPathDistanceMatrix		[]uint32
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	b.elemsPerThread1Dim = 4
	b.blockSize = 16
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "floydwarshall")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	
	numNodes := b.hNumNodes
	b.hOutputPathMatrix = make([]uint32, numNodes * numNodes)
	b.hOutputPathDistanceMatrix = make([]uint32, numNodes * numNodes)

	for i := 0; i < numNodes; i++ {
		iXWidth := i * numNodes
		b.hOutputPathDistanceMatrix[iXWidth + i] = 0;
	}

	for i := 0; i < numNodes; ++i)
    {
        for j := 0; j < i; ++j)
        {
            b.hOutputPathMatrix[i * numNodes + j] = i;
            b.hOutputPathMatrix[j * numNodes + i] = j;
        }
        b.hOutputPathMatrix[i * numNodes + i] = i;
	}

	fmt.Println("Distance matrix on CPU:")
	for i := 0; i < b.numNodes; i++ {
		for j := 0; j < b.numNodes; j++ {
			actual := b.hOutputPathDistanceMatrix[j*b.numNodes+i]
			fmt.Printf("%d ", actual)
		}
		fmt.Printf("\n")
	}
	
	if(sampleArgs->verify)
    {

		b.hVerificationPathMatrix = make([]uint32, numNodes * numNodes)
		b.hVerificationPathDistanceMatrix = make([]uint32, numNodes * numNodes)

		/*
        memcpy(verificationPathDistanceMatrix, pathDistanceMatrix,
               numNodes * numNodes * sizeof(cl_int));
		memcpy(verificationPathMatrix, pathMatrix, numNodes*numNodes*sizeof(cl_int));
		*/
	}
	
	b.driver.dOutputPathMatrix = b.driver.AllocateMemory(b.context, uint32(numNodes * numNodes))
	b.driver.dOutputPathDistanceMatrix = b.driver.AllocateMemory(b.context, uint32(numNodes * numNodes))
	
	b.driver.Distribute(b.context, b.dOutputPathMatrix, uint32(numNodes * numNodes), b.gpus)
	b.driver.Distribute(b.context, b.dOutputPathDistanceMatrix, uint32(numNodes * numNodes), b.gpus)

	b.driver.MemCopyH2D(b.context, b.dOutputPathMatrix, b.hOutputPathMatrix)
	b.driver.MemCopyH2D(b.context, b.dOutputPathDistanceMatrix, b.hOutputPathDistanceMatrix)
}

func (b *Benchmark) exec() {

	for _, queue := range b.queues {
		
		for k:=0; k<numNodes; k++{
			b.pass = k
			
			kernArg := FloydWarshallKernelArgs{
				b.dOutputPathMatrix,
				b.dOutputPathDistanceMatrix,
				b.numNodes,
				b.pass,
			}
			
			b.driver.EnqueueLaunchKernel(
				queue,
				b.kernel,
				//[3]uint32{uint32(wiWidth), wiHeight, 1},
				//[3]uint16{uint16(b.blockSize), uint16(b.blockSize), 1},
				&kernArg,
			)
		}
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputPathMatrix, b.dOutputPathMatrix)
	b.driver.MemCopyD2H(b.context, b.hOutputPathDistanceMatrix, b.dOutputPathDistanceMatrix)

	fmt.Println("Resulting path distance matrix:")
	for i := 0; i < b.numNodes; i++ {
		for j := 0; j < b.numNodes; j++ {
			actual := b.hOutputPathDistanceMatrix[j*b.numNodes+i]
			fmt.Printf("%d ", actual)
		}
		fmt.Printf("\n")
	}

}

func (b *Benchmark) Verify() {

	/*
	 * Floyd-Warshall with CPU
	 */

	var distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance uint32
    width := numNodes
    var yXwidth uint32

    for k := 0; k < numNodes; ++k
    {
        for y := 0; y < numNodes; ++y
        {
            yXwidth =  y*numNodes;
            for x := 0; x < numNodes; ++x
            {
                distanceYtoX = b.hVerificationPathDistanceMatrix[yXwidth + x];
                distanceYtoK = b.hVerificationPathDistanceMatrix[yXwidth + k];
                distanceKtoX = b.hVerificationPathDistanceMatrix[k * width + x];

                indirectDistance = distanceYtoK + distanceKtoX;

                if indirectDistance < distanceYtoX
                {
                    b.hVerificationPathDistanceMatrix[yXwidth + x] = indirectDistance;
                    b.hVerificationPathMatrix[yXwidth + x]         = k;
                }
            }
        }
    }


	/*
	 * Verifying the result
	 */
	
	fmt.Println("Verification path distance matrix:")
	for i := 0; i < b.numNodes; i++ {
		for j := 0; j < b.numNodes; j++ {
			actual := b.hVerificationPathDistanceMatrix[j*b.numNodes+i]
			fmt.Printf("%d ", actual)
		}
		fmt.Printf("\n")
	}

	log.Printf("Passed!\n")
}


