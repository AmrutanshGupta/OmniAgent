// backend-go/internal/db/mongo.go
package db

import (
	"context"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client

func InitMongo(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI))
	return err
}

func GetClient() *mongo.Client {
	return client
}

func GetUserByEmail(email string, cfg *config.Config) (*models.User, error) {
	dbName := cfg.MongoDBName

	collection := client.Database(dbName).Collection("users")
	var user models.User

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := collection.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func DeductBudget(email string, costUSD float64, cfg *config.Config) error {
	if costUSD <= 0 {
		return nil
	}
	dbName := cfg.MongoDBName
	collection := client.Database(dbName).Collection("users")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.UpdateOne(ctx,
		bson.M{"email": email},
		bson.M{"$inc": bson.M{"budget_usd": -costUSD}},
	)
	return err
}