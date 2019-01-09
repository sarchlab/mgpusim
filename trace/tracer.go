package trace

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/rs/xid"
)

// A Tracer provides the service to write simulation traces to a database
type Tracer struct {
	client     *mongo.Client
	collection *mongo.Collection
}

// Init initialize the connection to the database
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

	t.collection = t.client.Database(dbName).Collection("trace")
	_, err = t.collection.Indexes().CreateOne(ctx,
		mongo.IndexModel{
			Keys:bson.D{{Key:"id", Value:"hashed"}},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}

// CreateTask adds a task into the database
func (t *Tracer) CreateTask(task *Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := t.collection.InsertOne(ctx, task)
	if err != nil {
		log.Panic(err)
	}
}

// EndTask marks the end time of a task
func (t *Tracer) EndTask(taskID string, endTime float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := t.collection.UpdateOne(ctx,
		bson.D{{Key: "id", Value: taskID}},
		bson.D{{
			Key:   "$set",
			Value: bson.D{{Key: "end", Value: endTime}},
		}},
	)
	if err != nil {
		log.Panic(err)
	}
}
