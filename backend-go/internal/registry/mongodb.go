package registry

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	client *mongo.Client
	col    *mongo.Collection
}

func NewMongoDB(uri string) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	col := client.Database("omniagent").Collection("model_registry")

	// Create indices
	_, err = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "model_id", Value: 1},
				{Key: "provenance.snapshot_version", Value: -1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "provenance.freshness_state", Value: 1},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create indices: %w", err)
	}

	return &MongoDB{client: client, col: col}, nil
}

// InsertSnapshot saves a new ModelFact to the registry.
func (db *MongoDB) InsertSnapshot(ctx context.Context, fact ModelFact) error {
	_, err := db.col.InsertOne(ctx, fact)
	return err
}

// GetLKG (Last Known Good) fetches the latest valid snapshot for a model.
// An LKG is considered the most recent snapshot that was successfully ingested and not rejected as anomalous.
func (db *MongoDB) GetLKG(ctx context.Context, modelID string) (*ModelFact, error) {
	var fact ModelFact
	opts := options.FindOne().SetSort(bson.D{{Key: "provenance.snapshot_version", Value: -1}})
	err := db.col.FindOne(ctx, bson.M{"model_id": modelID}, opts).Decode(&fact)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No LKG found
		}
		return nil, err
	}
	return &fact, nil
}

// UpdateStaleness processes the lifecycle of all snapshots.
// age <= 12h: FRESH
// 12h < age <= 48h: STALE
// age > 48h: EXPIRED
func (db *MongoDB) UpdateStaleness(ctx context.Context) error {
	now := time.Now()
	threshold12h := now.Add(-12 * time.Hour)
	threshold48h := now.Add(-48 * time.Hour)

	// Set to EXPIRED if older than 48h
	_, err := db.col.UpdateMany(
		ctx,
		bson.M{"provenance.fetched_at": bson.M{"$lte": threshold48h}},
		bson.M{"$set": bson.M{"provenance.freshness_state": FreshnessExpired}},
	)
	if err != nil {
		return err
	}

	// Set to STALE if between 12h and 48h
	_, err = db.col.UpdateMany(
		ctx,
		bson.M{
			"provenance.fetched_at": bson.M{"$lte": threshold12h, "$gt": threshold48h},
		},
		bson.M{"$set": bson.M{"provenance.freshness_state": FreshnessStale}},
	)
	if err != nil {
		return err
	}

	// Set to FRESH if newer than 12h
	_, err = db.col.UpdateMany(
		ctx,
		bson.M{"provenance.fetched_at": bson.M{"$gt": threshold12h}},
		bson.M{"$set": bson.M{"provenance.freshness_state": FreshnessFresh}},
	)

	return err
}

// GetActiveSnapshots returns the latest FRESH or STALE snapshot for each model.
// If a model only has EXPIRED snapshots, we fall back to its LKG (which is the most recent EXPIRED).
func (db *MongoDB) GetActiveSnapshots(ctx context.Context) ([]ModelFact, error) {
	// Aggregate to get the latest snapshot per model
	pipeline := mongo.Pipeline{
		{{Key: "$sort", Value: bson.D{{Key: "provenance.snapshot_version", Value: -1}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$model_id"},
			{Key: "doc", Value: bson.D{{Key: "$first", Value: "$$ROOT"}}},
		}}},
		{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$doc"}}}},
	}
	
	cursor, err := db.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ModelFact
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	
	
	return results, nil
}

// UpdateObservedEMA applies an Exponential Moving Average update to latency and error rate.
func (db *MongoDB) UpdateObservedEMA(ctx context.Context, modelID string, latencyMs int64, qualityScore float64, alpha float64) error {
	var current struct {
		Observed ObservedMetrics `bson:"observed"`
	}
	
	err := db.col.FindOne(ctx, bson.M{"model_id": modelID}).Decode(&current)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil // Model might not exist yet, skip update
		}
		return err
	}
	
	newLatency := float64(latencyMs)*alpha + float64(current.Observed.P50LatencyMs)*(1.0-alpha)
	
	errSignal := 0.0
	if qualityScore < 0.2 {
		errSignal = 1.0
	}
	newErrRate := errSignal*alpha + current.Observed.ErrorRate5m*(1.0-alpha)
	
	_, err = db.col.UpdateMany(
		ctx,
		bson.M{"model_id": modelID},
		bson.M{
			"$set": bson.M{
				"observed.p50_latency_ms": int(newLatency),
				"observed.error_rate_5m":  newErrRate,
			},
		},
	)
	
	return err
}

func (db *MongoDB) Ping(ctx context.Context) error {
	return db.client.Ping(ctx, nil)
}

// ResetLKG deletes all stored snapshots for a model, clearing any anomaly-reject loop.
// The model will be treated as first-seen on the next ingestion cycle.
func (db *MongoDB) ResetLKG(ctx context.Context, modelID string) (int64, error) {
	res, err := db.col.DeleteMany(ctx, bson.M{"model_id": modelID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
