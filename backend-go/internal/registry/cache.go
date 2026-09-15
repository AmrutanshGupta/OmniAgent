package registry

import (
	"context"
	"github.com/rs/zerolog/log"
	"sync"
	"time"
)

type Cache struct {
	db *MongoDB

	mu       sync.RWMutex
	byID     map[string]ModelFact
	byTier   map[Tier][]ModelFact
}

func NewCache(db *MongoDB) *Cache {
	return &Cache{
		db:     db,
		byID:   make(map[string]ModelFact),
		byTier: make(map[Tier][]ModelFact),
	}
}

func (c *Cache) Start(ctx context.Context) {
	c.refresh(ctx)
	
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.refresh(ctx)
			}
		}
	}()
}

func (c *Cache) refresh(ctx context.Context) {
	// First update staleness in DB
	if err := c.db.UpdateStaleness(ctx); err != nil {
		log.Printf("[registry] Failed to update staleness: %v", err)
	}

	facts, err := c.db.GetActiveSnapshots(ctx)
	if err != nil {
		log.Printf("[registry] Cache refresh failed: %v", err)
		return
	}

	newByID := make(map[string]ModelFact)
	newByTier := make(map[Tier][]ModelFact)

	for _, f := range facts {
		newByID[f.ModelID] = f
		newByTier[f.Tier] = append(newByTier[f.Tier], f)
	}

	c.mu.Lock()
	c.byID = newByID
	c.byTier = newByTier
	c.mu.Unlock()
}

func (c *Cache) GetModel(modelID string) (ModelFact, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fact, ok := c.byID[modelID]
	return fact, ok
}

func (c *Cache) GetModelsByTier(tier Tier) []ModelFact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	facts := c.byTier[tier]
	// Return a copy so caller doesn't mutate cache slices
	cp := make([]ModelFact, len(facts))
	copy(cp, facts)
	return cp
}

// GetAllModels returns a snapshot copy of all active models in the cache.
func (c *Cache) GetAllModels() []ModelFact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]ModelFact, 0, len(c.byID))
	for _, f := range c.byID {
		res = append(res, f)
	}
	return res
}
