// Package sandbox provides a production-grade ephemeral multi-container
// sandboxing engine using Docker-out-of-Docker (DooD) for running and
// validating full-stack web applications.
package sandbox

import (
	"context"
	"fmt"
	"io"
	"github.com/rs/zerolog/log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func getWorkspaceRoot(cfg *config.Config) string {
	if cfg != nil && cfg.WorkspaceRoot != "" {
		return cfg.WorkspaceRoot
	}
	return "/tmp/omniagent/workspaces"
}

const (
	hardTimeout   = 30 * time.Second
	stopGraceSecs = 2
	memLimitBytes = int64(512 * 1024 * 1024) // 512 MB
	nanoCPULimit  = int64(1_000_000_000)     // 1.0 CPU core
	pidsLimitVal  = int64(128)
)

// ProjectFile represents a single file in a generated multi-file project.
type ProjectFile struct {
	// RelPath is relative to the workspace root, e.g. "src/index.js"
	RelPath  string
	Contents []byte
}

// DoodOrchestrator manages ephemeral containers via the Docker Engine API
// connected through the Docker socket proxy (DooD pattern).
type DoodOrchestrator struct {
	cli *client.Client
}

// NewDoodOrchestrator creates an orchestrator connecting via DOCKER_HOST env
// (pointed at the Tecnativa socket proxy) or the default Unix socket.
func NewDoodOrchestrator() (*DoodOrchestrator, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("sandbox: failed to create Docker client: %w", err)
	}
	return &DoodOrchestrator{cli: cli}, nil
}

// Close releases underlying Docker client resources.
func (o *DoodOrchestrator) Close() error {
	return o.cli.Close()
}

// WriteWorkspace materialises the generated file tree under
// /tmp/omniagent/workspaces/<taskID>/ and returns the absolute host path.
func (o *DoodOrchestrator) WriteWorkspace(taskID string, files []ProjectFile, cfg *config.Config) (string, error) {
	dir := filepath.Join(getWorkspaceRoot(cfg), taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox: mkdir workspace: %w", err)
	}
	for _, f := range files {
		destPath := filepath.Join(dir, f.RelPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", fmt.Errorf("sandbox: mkdir %s: %w", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, f.Contents, 0o644); err != nil {
			return "", fmt.Errorf("sandbox: write %s: %w", f.RelPath, err)
		}
	}
	log.Printf("[sandbox] workspace written: %s (%d files)", dir, len(files))
	return dir, nil
}

// RemoveWorkspace deletes the on-disk workspace directory unconditionally.
func RemoveWorkspace(taskID string, cfg *config.Config) {
	dir := filepath.Join(getWorkspaceRoot(cfg), taskID)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("[sandbox] warn: failed to remove workspace %s: %v", dir, err)
	} else {
		log.Printf("[sandbox] workspace removed: %s", dir)
	}
}

// createBridgeNetwork provisions an isolated internal Docker bridge network.
// Internal:true disables the external gateway — prevents SSRF and exfiltration.
func (o *DoodOrchestrator) createBridgeNetwork(ctx context.Context, taskID string) (string, error) {
	netName := "net-" + taskID
	resp, err := o.cli.NetworkCreate(ctx, netName, types.NetworkCreate{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			"omniagent.task_id": taskID,
			"omniagent.managed": "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("sandbox: create network %s: %w", netName, err)
	}
	log.Printf("[sandbox] network created: %s (id=%s)", netName, resp.ID[:12])
	return resp.ID, nil
}

// removeNetwork removes the ephemeral bridge network by ID.
func (o *DoodOrchestrator) removeNetwork(ctx context.Context, networkID string) {
	if err := o.cli.NetworkRemove(ctx, networkID); err != nil {
		f := filters.NewArgs(filters.Arg("label", "omniagent.managed=true"))
		if _, pErr := o.cli.NetworksPrune(ctx, f); pErr != nil {
			log.Printf("[sandbox] warn: could not remove network %s: %v", networkID[:12], err)
		}
	} else {
		log.Printf("[sandbox] network removed: %s", networkID[:12])
	}
}

// hardResourceConfig returns a HostConfig enforcing strict kernel security.
// workspaceDir is bind-mounted at /workspace. Only /tmp (tmpfs) is writable by default.
// Set writableWorkspace=true for the app server container that needs npm install.
func hardResourceConfig(workspaceDir string, writableWorkspace bool) *dockercontainer.HostConfig {
	pidLimit := pidsLimitVal
	cfg := &dockercontainer.HostConfig{
		Resources: dockercontainer.Resources{
			Memory:     memLimitBytes,
			MemorySwap: 0, // Disable swap entirely
			NanoCPUs:   nanoCPULimit,
			PidsLimit:  &pidLimit,
		},
		ReadonlyRootfs: !writableWorkspace,
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   workspaceDir,
				Target:   "/workspace",
				ReadOnly: !writableWorkspace,
			},
		},
		Tmpfs: map[string]string{
			"/tmp": "size=128m,mode=1777",
		},
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
	}
	return cfg
}

// stopAndRemoveContainer performs graceful stop → SIGKILL → force remove.
// Uses its own background context so cleanup always runs after cancellation.
func (o *DoodOrchestrator) stopAndRemoveContainer(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grace := stopGraceSecs
	if err := o.cli.ContainerStop(ctx, id, dockercontainer.StopOptions{Timeout: &grace}); err != nil {
		log.Printf("[sandbox] warn: stop %s: %v — sending SIGKILL", id[:12], err)
		_ = o.cli.ContainerKill(ctx, id, "SIGKILL")
	}
	if err := o.cli.ContainerRemove(ctx, id, dockercontainer.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		log.Printf("[sandbox] warn: remove %s: %v", id[:12], err)
	} else {
		log.Printf("[sandbox] container removed: %s", id[:12])
	}
}

// pullImageIfAbsent pulls an image only when it is not already cached locally.
func (o *DoodOrchestrator) pullImageIfAbsent(ctx context.Context, img string) error {
	_, _, err := o.cli.ImageInspectWithRaw(ctx, img)
	if err == nil {
		return nil // Already present locally
	}
	log.Printf("[sandbox] pulling image: %s", img)
	rc, err := o.cli.ImagePull(ctx, img, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("sandbox: pull %s: %w", img, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("sandbox: drain pull stream %s: %w", img, err)
	}
	return nil
}

// networkEndpoint returns a typed EndpointSettings for the given network.
func networkEndpoint(netID string, aliases ...string) *network.EndpointSettings {
	return &network.EndpointSettings{
		NetworkID: netID,
		Aliases:   aliases,
	}
}

// randomSuffix returns a 4-byte random hex string for unique resource naming.
func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b) //nolint:gosec
	return fmt.Sprintf("%x", b)
}
