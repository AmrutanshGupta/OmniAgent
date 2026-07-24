package main

import (
	"log"
	"os"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"llm-orchestrator/backend-go/internal/agents"
	"llm-orchestrator/backend-go/internal/models"
	"llm-orchestrator/backend-go/internal/security"
	"llm-orchestrator/backend-go/internal/ws"
)

func main() {
	hub := ws.NewHub()
	app := fiber.New(fiber.Config{AppName: "OmniAgent Orchestrator"})

	app.Use(cors.New())
	// Logger middleware format deliberately excludes request bodies so BYOK
	// keys (sent in POST /api/plan body) never touch stdout. See security.Redact.
	app.Use(logger.New(logger.Config{Format: "[${time}] ${method} ${path} -> ${status} (${latency})\n"}))

	app.Get("/api/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "backend-go"})
	})

	// Kick off a full Planner -> Worker -> Critic -> Synthesizer run.
	app.Post("/api/plan", func(c *fiber.Ctx) error {
		var req models.PlanRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.Prompt == "" {
			return c.Status(400).JSON(fiber.Map{"error": "prompt is required"})
		}
		log.Printf("plan request received (keys present: openai=%v anthropic=%v google=%v)",
			req.Keys.OpenAI != "", req.Keys.Anthropic != "", req.Keys.Google != "")

		plan := agents.RunPipeline(req, func(eventType string, data interface{}) {
			hub.Broadcast(ws.Event{Type: eventType, Data: data})
		})

		final := agents.Synthesize(plan.Tasks)
		spent, saved := agents.TotalCost(plan.Tasks)

		resp := fiber.Map{
			"plan":       plan,
			"final":      final,
			"costSpent":  spent,
			"costSaved":  saved,
		}
		hub.Broadcast(ws.Event{Type: "run_complete", Data: resp})
		return c.JSON(resp)
	})

	// BYOK vault demo endpoints (encrypt-on-write, never return plaintext).
	app.Post("/api/vault/encrypt", func(c *fiber.Ctx) error {
		var body struct{ Key string `json:"key"` }
		if err := c.BodyParser(&body); err != nil || body.Key == "" {
			return c.Status(400).JSON(fiber.Map{"error": "key required"})
		}
		enc, err := security.Encrypt(body.Key)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "encryption failed"})
		}
		return c.JSON(fiber.Map{"encrypted": enc, "preview": security.Redact(body.Key)})
	})

	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		hub.Register(c)
		defer hub.Unregister(c)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("OmniAgent Go orchestrator listening on :%s", port)
	log.Fatal(app.Listen(":" + port))
}
