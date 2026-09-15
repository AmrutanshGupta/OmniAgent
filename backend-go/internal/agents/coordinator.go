package agents

import (
	"context"
	"github.com/rs/zerolog/log"
	"strings"
	"sync"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/coordinator"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/critic"
	dbpkg "github.com/AmrutanshGupta/OmniAgent/backend-go/internal/db"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/pricing"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/router_client"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/ws"
	
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ProcessTask(ctx context.Context, prompt string, hub *ws.Hub, weights map[string]float64, cfg *config.Config, cache *registry.Cache) {
	userID, ok := ctx.Value(models.UserIDKey).(string)
	if !ok {
		return
	}
	sessionID, ok := ctx.Value(models.SessionIDKey).(string)
	if !ok {
		return
	}
	keys, ok := ctx.Value(models.APIKeysKey).(map[string]string)
	if !ok {
		keys = map[string]string{}
	}

	var rec *critic.Recorder
	var db *mongo.Database
	if cfg.MongoDBURI != "" {
		if client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI)); err == nil {
			db = client.Database("omniagent")
			rec = critic.NewRecorder(db, cfg)
		}
	}

	dag, err := GenerateDAG(ctx, prompt, cfg)
	if err != nil {
		hub.BroadcastTargeted(userID, sessionID, "ERROR", err.Error())
		return
	}

	for _, n := range dag.Nodes {
		n.Status = models.StatePending
	}

	engine := coordinator.NewEngine(dag, db)

	// Decouple client context
	bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	bgCtx = context.WithValue(bgCtx, models.UserIDKey, userID)
	bgCtx = context.WithValue(bgCtx, models.SessionIDKey, sessionID)

	engine.GetDAG() // Just to sync

	hub.BroadcastTargeted(userID, sessionID, "DAG_UPDATE", dag)

	doneChans := make(map[string]chan struct{})
	for id := range dag.Nodes {
		doneChans[id] = make(chan struct{})
	}

	deps := make(map[string][]string)
	for _, edge := range dag.Edges {
		from, to := edge[0], edge[1]
		deps[to] = append(deps[to], from)
	}

	var wg sync.WaitGroup

	for _, node := range dag.Nodes {
		wg.Add(1)
		go func(n *models.TaskNode) {
			defer wg.Done()
			defer close(doneChans[n.ID])

			// Wait for all dependencies to finish
			for _, depID := range deps[n.ID] {
				if ch, ok := doneChans[depID]; ok {
					<-ch
				}
			}

			// Budget Check before dispatch
			if user, err := dbpkg.GetUserByEmail(userID, cfg); err != nil || user.BudgetUSD <= 0 {
				log.Warn().Str("node", n.ID).Msg("[Coordinator] Halting: insufficient budget")
				_ = engine.TransitionNode(bgCtx, n.ID, models.StatePending, models.StateFailed, nil)
				return
			}

			_ = engine.TransitionNode(bgCtx, n.ID, models.StatePending, models.StateReady, nil)

			// Gather upstream context
			upstreamContext := ""
			for _, depID := range deps[n.ID] {
				if depNode, ok := dag.Nodes[depID]; ok && depNode.Status == models.StateSucceeded {
					upstreamContext += "\n--- Output from " + depNode.Task + " ---\n" + depNode.Result + "\n"
				}
			}

			nodePrompt := prompt
			if upstreamContext != "" {
				nodePrompt += "\n\nContext from previous steps:" + upstreamContext
			}

			estimatedTokens := len(n.Task) / 4
			if estimatedTokens < 1 {
				estimatedTokens = 1
			}

			costGPT4o := float64(estimatedTokens) * pricing.GetOutputCost("gpt-5.4", 0.000015)
			costSonnet := float64(estimatedTokens) * pricing.GetOutputCost("claude-sonnet-5", 0.000015)
			costFlash := float64(estimatedTokens) * pricing.GetOutputCost("gemini-3.6-flash", 0.00000015)

			routeReq := models.RouterRequest{
				PromptTokens:        estimatedTokens,
				ComplexityHeuristic: 5.0,
				TaskDomainIdx:       n.Domain,
				Q0:                  0,
				Q1:                  0,
				Q2:                  0,
				Q3:                  0,
				Q4:                  0,
				GPUUtil:             0.5,
				C0:                  costGPT4o,
				C1:                  costSonnet,
				C2:                  costFlash,
				C3:                  0.0,
				C4:                  0.0,
				WQ:                  weights["w_q"],
				WC:                  weights["w_c"],
				WL:                  weights["w_l"],
			}

			candidates := []registry.ModelFact{}
			if cache != nil {
				candidates = cache.GetAllModels()
			}
			mlTierInt := 2
			routeResp, err := router_client.GetOptimalRoute(routeReq, cfg, candidates)
			if err == nil && routeResp != nil {
				mlTierInt = routeResp.ChosenAgentID
			}

			actualTier := AssignTier(n.Domain, mlTierInt, keys, cfg)

			engine.UpdateTier(n.ID, actualTier)
			_ = engine.TransitionNode(bgCtx, n.ID, models.StateReady, models.StateRunning, map[string]string{"tier": actualTier})
			
			hub.BroadcastTargeted(userID, sessionID, "DAG_UPDATE", engine.GetDAG())

			var execErr error

			// Heuristic override: Ensure test generation and coding tasks are routed to the sandbox
			taskLower := strings.ToLower(n.Task)
			if n.Domain != 1 {
				sandboxKeywordsStr := cfg.SandboxKeywords
				if sandboxKeywordsStr == "" {
					sandboxKeywordsStr = "test,code,script"
				}
				for _, kw := range strings.Split(sandboxKeywordsStr, ",") {
					if strings.Contains(taskLower, strings.TrimSpace(kw)) {
						n.Domain = 1
						break
					}
				}
			}

			if n.Domain == 1 {
				execErr = RunCodingTask(bgCtx, n, actualTier, nodePrompt, rec, cfg)
			} else {
				execErr = ExecuteReActLoop(bgCtx, n, actualTier, nodePrompt, cfg)
			}
			// Deduct cost after execution
			if n.CostUSD > 0 {
				_ = dbpkg.DeductBudget(userID, n.CostUSD, cfg)
			}

			_ = engine.TransitionNode(bgCtx, n.ID, models.StateRunning, models.StateEvaluating, nil)
			criticPassed := CriticEvaluate(bgCtx, n.Result)
			log.Info().Str("node", n.ID).Err(execErr).Bool("critic_passed", criticPassed).Str("status", string(n.Status)).Int("result_len", len(n.Result)).Msg("[Coordinator] first pass done")

			if execErr != nil || !criticPassed {
				_ = engine.TransitionNode(bgCtx, n.ID, models.StateEvaluating, models.StateEscalated, nil)

				fallbackTier := "gpt-4o"
				
				tierOrderStr := cfg.EscalationTierOrder
				if tierOrderStr == "" {
					tierOrderStr = "gpt-4o,sonnet,flash,groq,hf"
				}
				tierOrder := strings.Split(tierOrderStr, ",")
				
				for _, t := range tierOrder {
					t = strings.TrimSpace(t)
					if t == actualTier {
						continue // Don't escalate to the same tier
					}
					
					hasKey := false
					switch t {
					case "gpt-4o": if keys["openai"] != "" { hasKey = true }
					case "sonnet": if keys["anthropic"] != "" { hasKey = true }
					case "flash": if keys["google"] != "" { hasKey = true }
					case "groq": if keys["groq"] != "" { hasKey = true }
					case "hf": if keys["huggingface"] != "" { hasKey = true }
					}
					
					if hasKey || t == "hf" || t == "groq" {
						fallbackTier = t
						break
					}
				}

				engine.UpdateTier(n.ID, fallbackTier)
				hub.BroadcastTargeted(userID, sessionID, "DAG_UPDATE", engine.GetDAG())

				log.Warn().Str("node", n.ID).Str("from_tier", actualTier).Str("to_tier", fallbackTier).Err(execErr).Bool("critic_passed", criticPassed).Msg("[Coordinator] ESCALATING")

				_ = engine.TransitionNode(bgCtx, n.ID, models.StateEscalated, models.StateRunning, nil)

				if n.Domain == 1 {
					_ = RunCodingTask(bgCtx, n, fallbackTier, nodePrompt, rec, cfg)
				} else {
					_ = ExecuteReActLoop(bgCtx, n, fallbackTier, nodePrompt, cfg)
				}
				// Deduct cost after execution
				if n.CostUSD > 0 {
					_ = dbpkg.DeductBudget(userID, n.CostUSD, cfg)
				}
				
				_ = engine.TransitionNode(bgCtx, n.ID, models.StateRunning, models.StateEvaluating, nil)
			}

			if execErr != nil || !criticPassed {
				_ = engine.TransitionNode(bgCtx, n.ID, models.StateEvaluating, models.StateFailed, nil)
			} else {
				_ = engine.TransitionNode(bgCtx, n.ID, models.StateEvaluating, models.StateSucceeded, nil)
			}

			hub.BroadcastTargeted(userID, sessionID, "DAG_UPDATE", engine.GetDAG())
		}(node)
	}

	wg.Wait()
	finalOutput := Synthesize(engine.GetDAG())
	spent, saved := TotalCost(engine.GetDAG())

	hub.BroadcastTargeted(userID, sessionID, "FINAL_RESULT", finalOutput)
	hub.BroadcastTargeted(userID, sessionID, "TELEMETRY_UPDATE", map[string]interface{}{
		"activeTier":  "Synthesis Complete",
		"currentCost": spent,
		"tokensSaved": saved,
	})
}