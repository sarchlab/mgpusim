package sim

import (
	"log"
	"sync"
	"sync/atomic"
)

var idGeneratorMutex sync.Mutex
var idGeneratorInstantiated bool
var idGenerator IDGenerator

// IDGenerator can generate IDs
type IDGenerator interface {
	// Generate an ID
	Generate() uint64
}

// UseSequentialIDGenerator configures the ID generator to generate IDs in
// sequential.
func UseSequentialIDGenerator() {
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGeneratorMutex.Lock()

	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGenerator = &sequentialIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// UseParallelIDGenerator configurs the ID generator to generate ID in
// parallel. The IDs generated will not be deterministic anymore.
func UseParallelIDGenerator() {
	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGeneratorMutex.Lock()

	if idGeneratorInstantiated {
		log.Panic("cannot change id generator type after using it")
	}

	idGenerator = &parallelIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()
}

// GetIDGenerator returns the ID generator used in the current simulation
func GetIDGenerator() IDGenerator {
	if idGeneratorInstantiated {
		return idGenerator
	}

	idGeneratorMutex.Lock()

	if idGeneratorInstantiated {
		idGeneratorMutex.Unlock()
		return idGenerator
	}

	idGenerator = &sequentialIDGenerator{}
	idGeneratorInstantiated = true

	idGeneratorMutex.Unlock()

	return idGenerator
}

type sequentialIDGenerator struct {
	nextID uint64
}

func (g *sequentialIDGenerator) Generate() uint64 {
	return atomic.AddUint64(&g.nextID, 1)
}

// GetIDGeneratorNextID returns the current nextID from the sequential ID
// generator. It panics if the ID generator is not a sequentialIDGenerator.
func GetIDGeneratorNextID() uint64 {
	gen := idGenerator.(*sequentialIDGenerator)
	return atomic.LoadUint64(&gen.nextID)
}

// SetIDGeneratorNextID sets the nextID on the sequential ID generator.
// It panics if the ID generator is not a sequentialIDGenerator.
func SetIDGeneratorNextID(id uint64) {
	gen := idGenerator.(*sequentialIDGenerator)
	atomic.StoreUint64(&gen.nextID, id)
}

// ResetIDGenerator resets the ID generator so a new one can be created.
func ResetIDGenerator() {
	idGeneratorInstantiated = false
	idGenerator = nil
}

type parallelIDGenerator struct {
	nextID uint64
}

func (g *parallelIDGenerator) Generate() uint64 {
	return atomic.AddUint64(&g.nextID, 1)
}
