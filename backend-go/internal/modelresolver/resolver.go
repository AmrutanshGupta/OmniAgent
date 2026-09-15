package modelresolver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tier string

const (
	TierFlash  Tier = "flash"
	TierSonnet Tier = "sonnet"
	TierGPT4o  Tier = "gpt-4o"
	TierGroq   Tier = "groq"
	TierHF     Tier = "hf"
)

type ResolvedModel struct {
	Provider   string
	Model      string
	Source     string
	ResolvedAt time.Time
}

type Config struct {
	OpenAIKey    string
	AnthropicKey string
	GoogleKey    string
	GroqKey      string

	HTTPClient *http.Client

	RefreshInterval time.Duration
}

type tierRule struct {
	provider string
	fetch    func(r *Resolver, ctx context.Context) ([]candidate, error)
	match    *regexp.Regexp
	exclude  []string
	fallback string
	rank     func(id string) float64
}

type candidate struct {
	ID        string
	CreatedAt *time.Time
}

func defaultRules() map[Tier]tierRule {
	return map[Tier]tierRule{
		TierFlash: {
			provider: "google",
			fetch:    (*Resolver).fetchGoogleModels,
			match:    regexp.MustCompile(`^gemini-(\d+)(\.\d+)?-flash$`),
			exclude:  []string{"lite", "image", "preview", "live", "audio"},
			fallback: "gemini-2.5-flash",
			rank:     geminiVersionRank,
		},
		TierSonnet: {
			provider: "anthropic",
			fetch:    (*Resolver).fetchAnthropicModels,
			match:    regexp.MustCompile(`sonnet`),
			exclude:  []string{},
			fallback: "claude-sonnet-5",
			rank:     nil,
		},
		TierGPT4o: {
			provider: "openai",
			fetch:    (*Resolver).fetchOpenAIModels,
			match:    regexp.MustCompile(`^gpt-\d+(\.\d+)*$`),
			exclude:  []string{"mini", "nano", "preview", "instruct", "audio", "search", "chat-latest"},
			fallback: "gpt-5.4",
			rank:     dottedVersionRank,
		},
		TierGroq: {
			provider: "groq",
			fetch:    (*Resolver).fetchGroqModels,
			match:    regexp.MustCompile(`^openai/gpt-oss-\d+b$`),
			exclude:  []string{},
			fallback: "openai/gpt-oss-120b",
			rank:     paramSizeRank,
		},
	}
}

type Resolver struct {
	cfg   Config
	rules map[Tier]tierRule

	mu    sync.RWMutex
	cache map[Tier]ResolvedModel
}

func New(cfg Config) *Resolver {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 12 * time.Hour
	}
	return &Resolver{
		cfg:   cfg,
		rules: defaultRules(),
		cache: map[Tier]ResolvedModel{},
	}
}

func (r *Resolver) Start(ctx context.Context) {
	r.refreshAll(ctx)
	go func() {
		t := time.NewTicker(r.cfg.RefreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.refreshAll(ctx)
			}
		}
	}()
}

func (r *Resolver) Resolve(tier Tier) ResolvedModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rm, ok := r.cache[tier]; ok {
		return rm
	}
	rule := r.rules[tier]
	return ResolvedModel{Provider: rule.provider, Model: rule.fallback, Source: "fallback", ResolvedAt: time.Now()}
}

func (r *Resolver) refreshAll(ctx context.Context) {
	var wg sync.WaitGroup
	for tier, rule := range r.rules {
		wg.Add(1)
		go func(tier Tier, rule tierRule) {
			defer wg.Done()
			r.refreshOne(ctx, tier, rule)
		}(tier, rule)
	}
	wg.Wait()
}

func (r *Resolver) refreshOne(ctx context.Context, tier Tier, rule tierRule) {
	candidates, err := rule.fetch(r, ctx)
	if err != nil {
		log.Printf("modelresolver: %s tier %q refresh failed, keeping cached/fallback: %v", rule.provider, tier, err)
		return
	}

	filtered := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if !rule.match.MatchString(c.ID) {
			continue
		}
		lower := strings.ToLower(c.ID)
		excluded := false
		for _, ex := range rule.exclude {
			if strings.Contains(lower, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		filtered = append(filtered, c)
	}

	if len(filtered) == 0 {
		log.Printf("modelresolver: %s tier %q — no candidates matched filters this refresh, keeping cached/fallback", rule.provider, tier)
		return
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if rule.rank != nil {
			return rule.rank(filtered[i].ID) > rule.rank(filtered[j].ID)
		}
		if filtered[i].CreatedAt != nil && filtered[j].CreatedAt != nil {
			return filtered[i].CreatedAt.After(*filtered[j].CreatedAt)
		}
		return false
	})

	winner := filtered[0].ID

	r.mu.Lock()
	prev, existed := r.cache[tier]
	r.cache[tier] = ResolvedModel{Provider: rule.provider, Model: winner, Source: "live", ResolvedAt: time.Now()}
	r.mu.Unlock()

	if !existed || prev.Model != winner {
		log.Printf("modelresolver: tier %q (%s) -> %s (was %q)", tier, rule.provider, winner, prev.Model)
	}
}

func (r *Resolver) fetchOpenAIModels(ctx context.Context) ([]candidate, error) {
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := r.getJSON(ctx, "https://api.openai.com/v1/models", map[string]string{
		"Authorization": "Bearer " + r.cfg.OpenAIKey,
	}, &out); err != nil {
		return nil, err
	}
	res := make([]candidate, 0, len(out.Data))
	for _, m := range out.Data {
		t := time.Unix(m.Created, 0)
		res = append(res, candidate{ID: m.ID, CreatedAt: &t})
	}
	return res, nil
}

func (r *Resolver) fetchAnthropicModels(ctx context.Context) ([]candidate, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := r.getJSON(ctx, "https://api.anthropic.com/v1/models", map[string]string{
		"x-api-key":         r.cfg.AnthropicKey,
		"anthropic-version": "2023-06-01",
	}, &out); err != nil {
		return nil, err
	}
	res := make([]candidate, 0, len(out.Data))
	for _, m := range out.Data {
		res = append(res, candidate{ID: m.ID})
	}
	return res, nil
}

func (r *Resolver) fetchGoogleModels(ctx context.Context) ([]candidate, error) {
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + r.cfg.GoogleKey
	if err := r.getJSON(ctx, url, nil, &out); err != nil {
		return nil, err
	}
	res := make([]candidate, 0, len(out.Models))
	for _, m := range out.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		res = append(res, candidate{ID: id})
	}
	return res, nil
}

func (r *Resolver) fetchGroqModels(ctx context.Context) ([]candidate, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := r.getJSON(ctx, "https://api.groq.com/openai/v1/models", map[string]string{
		"Authorization": "Bearer " + r.cfg.GroqKey,
	}, &out); err != nil {
		return nil, err
	}
	res := make([]candidate, 0, len(out.Data))
	for _, m := range out.Data {
		res = append(res, candidate{ID: m.ID})
	}
	return res, nil
}

func (r *Resolver) getJSON(ctx context.Context, url string, headers map[string]string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := r.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var dottedVersionRe = regexp.MustCompile(`(\d+(?:\.\d+)*)`)

func dottedVersionRank(id string) float64 {
	m := dottedVersionRe.FindString(id)
	if m == "" {
		return 0
	}
	parts := strings.Split(m, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return float64(major) + float64(minor)/1000.0
}

func geminiVersionRank(id string) float64 {
	return dottedVersionRank(id)
}

var paramSizeRe = regexp.MustCompile(`(\d+)b`)

func paramSizeRank(id string) float64 {
	m := paramSizeRe.FindStringSubmatch(strings.ToLower(id))
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return float64(n)
}