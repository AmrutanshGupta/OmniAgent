package critic

import (
	"context"
	"github.com/rs/zerolog/log"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type ExecutionTelemetry struct {
	ModelID       string    `bson:"model_id"`
	SessionID     string    `bson:"session_id"`
	NodeID        string    `bson:"node_id"`
	QualityScore  float64   `bson:"quality_score"`
	LatencyMs     int64     `bson:"latency_ms"`
	CostUSD       float64   `bson:"cost_usd"`
	AttemptNumber int       `bson:"attempt_number"`
	RecordedAt    time.Time `bson:"recorded_at"`
}

type Recorder struct {
	telCol   *mongo.Collection
	regCol   *mongo.Collection
	emaAlpha float64
}

func NewRecorder(db *mongo.Database, cfg *config.Config) *Recorder {
	// Create index
	telCol := db.Collection("execution_telemetry")
	_, err := telCol.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "model_id", Value: 1}, {Key: "recorded_at", Value: -1}},
	})
	if err != nil {
		log.Warn().Err(err).Msg("[Recorder] failed to create index")
	}

	alpha := cfg.EmaAlpha
	if alpha == 0.0 {
		alpha = 0.1
	}

	return &Recorder{
		telCol:   telCol,
		regCol:   db.Collection("model_registry"),
		emaAlpha: alpha,
	}
}

// RegistryUpdater defines the interface to decouple critic from registry.MongoDB
type RegistryUpdater interface {
	UpdateObservedEMA(ctx context.Context, modelID string, latencyMs int64, qualityScore float64, alpha float64) error
}

func (r *Recorder) Record(ctx context.Context, tel ExecutionTelemetry, reg RegistryUpdater) error {
	tel.RecordedAt = time.Now()
	_, err := r.telCol.InsertOne(ctx, tel)
	if err != nil {
		return err
	}

	if reg != nil {
		if err := reg.UpdateObservedEMA(ctx, tel.ModelID, tel.LatencyMs, tel.QualityScore, r.emaAlpha); err != nil {
			log.Warn().Err(err).Msg("[Recorder] EMA update failed")
		}
	}
	return nil
}
