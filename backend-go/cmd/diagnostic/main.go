package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/db"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("=== OmniAgent Diagnostic CLI ===")
	_ = godotenv.Load("../../.env")
	
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("[FAIL] Config Load: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[PASS] Configuration loaded.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Print("Pinging MongoDB... ")
	if err := db.InitMongo(cfg); err != nil {
		fmt.Printf("[FAIL] Connection error: %v\n", err)
		os.Exit(1)
	}
	if err := db.GetClient().Ping(ctx, nil); err != nil {
		fmt.Printf("[FAIL] %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[PASS]")

	fmt.Print("Pinging Registry DB... ")
	regDB, err := registry.NewMongoDB(cfg.MongoDBURI)
	if err != nil {
		fmt.Printf("[FAIL] Connection error: %v\n", err)
		os.Exit(1)
	}
	if err := regDB.Ping(ctx); err != nil {
		fmt.Printf("[FAIL] %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[PASS]")

	fmt.Println("=== Diagnostic Check Complete ===")
}
