package main

import (
	"context"
	"github.com/rs/zerolog/log"
	"sync"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/agents"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/db"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/llm_clients"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/pricing"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/security"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/ws"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

var (
	pendingDAGs    = make(map[string]*models.DAG)
	pendingPrompts = make(map[string]string)
	dagMutex       sync.Mutex
)

func main() {
	log.Info().Msg("[System] Starting OmniAgent Go Backend...")

	err := godotenv.Overload()
	if err != nil {
		log.Warn().Err(err).Msg("[System] Could not load .env file")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Msgf("Failed to load configuration: %v", err)
	}

	// 1. Init Database
	log.Info().Msg("[System] Connecting to MongoDB...")
	if err := db.InitMongo(cfg); err != nil {
		log.Fatal().Msgf("Failed to connect to MongoDB: %v", err)
	}
	
	// Fatal startup blocker: Ping MongoDB
	if err := db.GetClient().Ping(context.Background(), nil); err != nil {
		log.Fatal().Msgf("MongoDB Ping failed: %v", err)
	}

	// 2. Initialize Model Registry
	regDB, err := registry.NewMongoDB(cfg.MongoDBURI)
	if err != nil {
		log.Fatal().Msgf("Failed to connect to Registry MongoDB: %v", err)
	}
	
	// Fatal startup blocker: Ping Registry DB
	if err := regDB.Ping(context.Background()); err != nil {
		log.Fatal().Msgf("Registry DB Ping failed: %v", err)
	}

	// Initialize Security Service
	credService := security.NewCredentialService(cfg.EncryptionKey)

	// Start Background Ingestion
	ingestWorker := registry.NewIngestionWorker(regDB)
	ingestWorker.Start(context.Background(), cfg.CronSchedule)

	// Start In-Memory Cache
	cache := registry.NewCache(regDB)
	cache.Start(context.Background())
	llm_clients.InitResolver(context.Background(), cache, credService)

	// 3. Initialize dynamic pricing scraper
	log.Info().Msg("[System] Initializing Pricing Cache...")
	pricing.InitPricingCache()

	app := fiber.New()
	hub := ws.NewHub()
	go hub.Run()

	outboxDB := db.GetClient().Database(cfg.MongoDBName)
	outboxPoller := ws.NewOutboxPoller(outboxDB, hub)
	outboxPoller.Start(context.Background())

	app.Get("/health/liveness", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	app.Get("/health/readiness", func(c *fiber.Ctx) error {
		if err := db.GetClient().Ping(context.Background(), nil); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("MongoDB unreachable")
		}
		if err := regDB.Ping(context.Background()); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Registry DB unreachable")
		}
		return c.SendStatus(fiber.StatusOK)
	})

	// Admin: reset a stuck model's LKG so it re-ingests cleanly on the next cycle.
	// Usage: DELETE /admin/registry/<model_id>  e.g. /admin/registry/upstage/solar-pro4
	app.Delete("/admin/registry/*", func(c *fiber.Ctx) error {
		modelID := c.Params("*")
		if modelID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "model_id is required"})
		}
		deleted, err := regDB.ResetLKG(context.Background(), modelID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		log.Warn().Str("model", modelID).Int64("deleted_snapshots", deleted).Msg("[admin] LKG reset")
		return c.JSON(fiber.Map{"model_id": modelID, "deleted_snapshots": deleted})
	})

	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			tokenString := c.Query("token")
			if tokenString == "" {
				return fiber.ErrUnauthorized
			}

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				return []byte(cfg.NextAuthSecret), nil
			})

			if err != nil || !token.Valid {
				return fiber.ErrUnauthorized
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || claims["email"] == nil {
				return fiber.ErrUnauthorized
			}

			c.Locals("email", claims["email"].(string))
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		email := c.Locals("email").(string)
		sessionID := uuid.New().String()

		log.Info().Str("email", email).Str("session", sessionID).Msg("[Orchestrator] User connected")

		hub.Register <- ws.RegisterPayload{
			Conn: c,
			Info: &ws.ClientInfo{UserID: email, SessionID: sessionID},
		}
		defer func() {
			log.Info().Str("email", email).Msg("[Orchestrator] User disconnected")
			hub.Unregister <- c
		}()

		user, err := db.GetUserByEmail(email, cfg)
		if err != nil {
			c.WriteJSON(map[string]interface{}{"type": "ERROR", "payload": "User not found."})
			return
		}

		apiKeys := map[string]string{
			"openai":      user.Keys.OpenAI,
			"anthropic":   user.Keys.Anthropic,
			"google":      user.Keys.Google,
			"groq":        user.Keys.Groq,
			"huggingface": user.Keys.HuggingFace,
		}

		// Inject Correlation ID (SessionID) into logger context for this connection (using zerolog)
		// We'll skip strict logger overriding for standard log alias here, but sessionID is available in context.

		ctx := context.WithValue(context.Background(), models.UserIDKey, email)
		ctx = context.WithValue(ctx, models.SessionIDKey, sessionID)
		ctx = context.WithValue(ctx, models.APIKeysKey, apiKeys)

		var msg models.ClientMessage
		for {
			if err := c.ReadJSON(&msg); err != nil {
				break
			}

			switch msg.Type {
			case "NEW_TASK":
				if msg.Payload == "" {
					continue
				}

				log.Info().Str("prompt", msg.Payload).Msg("[Orchestrator] NEW_TASK received")

				dag, err := agents.GenerateDAG(ctx, msg.Payload, cfg)
				if err != nil {
					log.Error().Err(err).Msg("[Orchestrator] DAG generation failed")
					c.WriteJSON(map[string]interface{}{"type": "ERROR", "payload": err.Error()})
					continue
				}

				log.Info().Int("nodes", len(dag.Nodes)).Msg("[Orchestrator] DAG generated, awaiting approval")

				dagMutex.Lock()
				pendingDAGs[sessionID] = dag
				pendingPrompts[sessionID] = msg.Payload
				dagMutex.Unlock()

				c.WriteJSON(map[string]interface{}{
					"type":    "DAG_PENDING_APPROVAL",
					"payload": dag,
				})

			case "APPROVE_DAG":
				log.Info().Str("session", sessionID).Msg("[Orchestrator] APPROVE_DAG")

				dagMutex.Lock()
				_, exists := pendingDAGs[sessionID]
				approvedPrompt := pendingPrompts[sessionID]
				if exists {
					delete(pendingDAGs, sessionID)
					delete(pendingPrompts, sessionID)
				}
				dagMutex.Unlock()

				if exists {
					c.WriteJSON(map[string]interface{}{
						"type":    "SYSTEM_MESSAGE",
						"payload": "Execution Authorized. Initiating agent cascade...",
					})
					log.Info().Msg("[Orchestrator] Dispatching ProcessTask")
					go agents.ProcessTask(ctx, approvedPrompt, hub, msg.Weights, cfg, cache)
				} else {
					log.Warn().Msg("[Orchestrator] APPROVE_DAG with no pending DAG")
				}

			case "REJECT_DAG":
				log.Info().Str("session", sessionID).Msg("[Orchestrator] REJECT_DAG")

				dagMutex.Lock()
				delete(pendingDAGs, sessionID)
				delete(pendingPrompts, sessionID)
				dagMutex.Unlock()
				c.WriteJSON(map[string]interface{}{
					"type":    "SYSTEM_MESSAGE",
					"payload": "Execution aborted by user. Awaiting new instructions.",
				})
			}
		}
	}))

	log.Info().Str("port", cfg.Port).Msg("[System] Server listening")
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal().Err(err).Msg("Server failed")
	}
}