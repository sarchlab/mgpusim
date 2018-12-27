package trace

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/rs/xid"
)

type Tracer struct {
	client     *mongo.Client
	collection *mongo.Collection
}

func (t *Tracer) Init() {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.client, err = mongo.Connect(ctx, "mongodb://localhost:27017")
	if err != nil {
		log.Panic(err)
	}

	dbName := xid.New().String()
	fmt.Printf("Trace collected in db %s\n", dbName)

	t.collection = t.client.Database(dbName).Collection("events")
}

func (t *Tracer) Trace(step Step) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := t.collection.InsertOne(ctx, step)
	if err != nil {
		log.Panic(err)
	}
}
