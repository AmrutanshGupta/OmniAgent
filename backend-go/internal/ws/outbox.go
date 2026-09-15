package ws

import (
	"context"
	"github.com/rs/zerolog/log"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type OutboxPoller struct {
	db  *mongo.Database
	hub *Hub
}

func NewOutboxPoller(db *mongo.Database, hub *Hub) *OutboxPoller {
	return &OutboxPoller{
		db:  db,
		hub: hub,
	}
}

func (p *OutboxPoller) Start(ctx context.Context) {
	if p.db == nil {
		return
	}
	
	// Create index on published flag for fast polling
	_, _ = p.db.Collection("outbox_events").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "published", Value: 1}, {Key: "timestamp", Value: 1}},
	})

	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pollEvents(ctx)
			}
		}
	}()
}

func (p *OutboxPoller) pollEvents(ctx context.Context) {
	col := p.db.Collection("outbox_events")
	
	// Find top 100 unpublished events ordered by timestamp
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}}).SetLimit(100)
	cursor, err := col.Find(ctx, bson.M{"published": bson.M{"$ne": true}}, opts)
	if err != nil {
		log.Printf("[Outbox] Polling error: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var events []struct {
		models.TaskEvent `bson:",inline"`
		Published        bool `bson:"published"`
	}
	if err := cursor.All(ctx, &events); err != nil {
		return
	}

	for _, ev := range events {
		// Broadcast via WebSocket
		p.hub.BroadcastTargeted(ev.UserID, ev.SessionID, string(ev.Type), ev.Payload)

		// Mark as delivered
		_, err := col.UpdateOne(ctx, bson.M{"_id": ev.EventID}, bson.M{"$set": bson.M{"published": true}})
		if err != nil {
			log.Printf("[Outbox] Failed to mark event %s as published: %v", ev.EventID, err)
		}
	}
}
