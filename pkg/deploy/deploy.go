// Package deploy implements the end-to-end deployment sequence for a single
// recipe: resolve → probe engine → ensure network → allocate ports →
// create container → seed config files → start container → persist state →
// print outputs.
package deploy

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	dockernat "github.com/docker/go-connections/nat"
	"github.com/google/uuid"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/ports"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
)

// containerOpTimeout is the maximum time for container create/start calls.
const containerOpTimeout = 2 * time.Minute

// Options controls a single deploy invocation.
type Options struct {
	// RecipeName is the recipe to resolve and deploy.
	RecipeName string

	// Scope overrides the recipe's declared scope (project | system | custom).
	// When empty the recipe's own metadata.scope is used.
	Scope string

	// EnvOverrides are KEY=VALUE pairs that override container environment
	// variables declared by the recipe.
	EnvOverrides []string

	// Addons is the list of optional addon names to deploy alongside the main
	// container, e.g. []string{"admin"}.
	Addons []string

	// ProjectDir is the current working directory, used to derive the
	// project-scoped network name. Defaults to os.Getwd() when empty.
	ProjectDir string
}

// Run executes the full deploy sequence and prints progress + outputs to
// stdout. It is the implementation behind `lbmesh up`.
func Run(opts Options) error {
	// ── 1. Resolve recipe ────────────────────────────────────────────────────
	fmt.Printf("Resolving recipe %q...\n", opts.RecipeName)
	resolved, err := recipe.Resolve(opts.RecipeName)
	if err != nil {
		return err
	}
	rec := &resolved.Recipe
	fmt.Printf("  source: %s (%s)\n", resolved.Source, resolved.Path)

	// ── 2. Determine scope ───────────────────────────────────────────────────
	scope := rec.Metadata.Scope
	if opts.Scope != "" {
		scope = opts.Scope
	}
	sockScope, err := toSocketScope(scope)
	if err != nil {
		return err
	}

	projectDir := opts.ProjectDir
	if sockScope == socket.ScopeProject && projectDir == "" {
		projectDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolving project directory: %w", err)
		}
	}

	// ── 3. Open state DB ─────────────────────────────────────────────────────
	database, err := db.OpenDefaultAndMigrate()
	if err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	defer database.Close()

	// ── 4. Probe container engine ────────────────────────────────────────────
	fmt.Println("Detecting container engine...")
	c, host, err := socket.Detect()
	if err != nil {
		return err
	}
	defer c.Close()
	fmt.Printf("  engine: %s\n", host)

	// ── 5. Ensure network ────────────────────────────────────────────────────
	fmt.Printf("Ensuring %s-scope network...\n", scope)
	netInfo, err := socket.ResolveNetwork(c, sockScope, projectDir, "")
	if err != nil {
		return err
	}
	fmt.Printf("  network: %s (%s)\n", netInfo.Name, netInfo.ID[:12])

	// ── 6. Pull image if needed ──────────────────────────────────────────────
	if err := ensureImage(c, rec.Container.Image); err != nil {
		return err
	}

	// ── 7. Build & deploy the main container ─────────────────────────────────
	mainID, err := deployContainer(c, database, netInfo, sockScope, rec.Metadata.Name,
		&rec.Container, rec.Resources, scope, parseEnvOverrides(opts.EnvOverrides), projectDir)
	if err != nil {
		return fmt.Errorf("deploying main container: %w", err)
	}

	// ── 8. Deploy requested addons ───────────────────────────────────────────
	for _, addonName := range opts.Addons {
		addon := rec.Addon(addonName)
		if addon == nil {
			return fmt.Errorf("recipe %q has no addon named %q", opts.RecipeName, addonName)
		}
		fmt.Printf("Deploying addon %q...\n", addonName)
		if err := ensureImage(c, addon.Container.Image); err != nil {
			return err
		}
		_, err := deployContainer(c, database, netInfo, sockScope,
			rec.Metadata.Name+"-"+addonName,
			&addon.Container, addon.Resources, scope, nil, projectDir)
		if err != nil {
			return fmt.Errorf("deploying addon %q: %w", addonName, err)
		}
		if addon.Outputs.Instructions != "" {
			fmt.Println()
			fmt.Println(addon.Outputs.Instructions)
		}
	}

	// ── 9. Register the network in state DB ──────────────────────────────────
	if err := socket.RegisterScopeNetwork(database, netInfo, sockScope, mainID, mainID); err != nil {
		// Non-fatal: state tracking failure should not abort a successful deploy.
		fmt.Fprintf(os.Stderr, "warning: registering network state: %v\n", err)
	}

	// ── 10. Print outputs ────────────────────────────────────────────────────
	printOutputs(rec.Outputs)

	return nil
}

// ---------------------------------------------------------------------------
// Container deploy helpers
// ---------------------------------------------------------------------------

// deployContainer creates, seeds files into, and starts a single container.
// It returns the full container ID.
func deployContainer(
	c *client.Client,
	database *sql.DB,
	netInfo socket.NetworkInfo,
	scope socket.Scope,
	name string,
	ctr *recipe.Container,
	res recipe.Resources,
	scopeStr string,
	envOverrides map[string]string,
	projectDir string,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), containerOpTimeout)
	defer cancel()

	serviceID := uuid.New().String()

	// ── Allocate ports ───────────────────────────────────────────────────────
	broker := ports.New(database)
	portReqs := make([]ports.Request, len(ctr.Ports))
	for i, p := range ctr.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		portReqs[i] = ports.Request{
			ServiceID:         serviceID,
			ContainerPort:     p.Container,
			PreferredHostPort: p.Host,
			Protocol:          proto,
		}
	}
	portResults, err := broker.AllocateMany(portReqs)
	if err != nil {
		return "", fmt.Errorf("allocating ports: %w", err)
	}

	// ── Build Docker config ──────────────────────────────────────────────────
	env := mergeEnv(ctr.Environment, envOverrides)
	portSet, portBindings := buildPortMappings(portResults)

	mounts := make([]mount.Mount, len(ctr.Volumes))
	for i, v := range ctr.Volumes {
		volName := volumeName(name, v)
		mounts[i] = mount.Mount{
			Type:   mount.TypeVolume,
			Source: volName,
			Target: v,
		}
	}

	containerName := "lbmesh-" + name

	cfg := &container.Config{
		Image:        ctr.Image,
		Env:          env,
		ExposedPorts: portSet,
		Labels: map[string]string{
			"lbmesh.managed":  "true",
			"lbmesh.recipe":   name,
			"lbmesh.scope":    scopeStr,
			"lbmesh.service":  serviceID,
		},
	}
	if len(ctr.Command) > 0 {
		cfg.Entrypoint = ctr.Command
	}
	if len(ctr.Args) > 0 {
		cfg.Cmd = ctr.Args
	}

	hostCfg := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
		NetworkMode:  container.NetworkMode(netInfo.Name),
	}

	fmt.Printf("Creating container %q...\n", containerName)
	created, err := c.ContainerCreate(ctx, cfg, hostCfg, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("creating container %q: %w", containerName, err)
	}
	cid := created.ID

	// ── Seed config files ────────────────────────────────────────────────────
	if err := seedFiles(ctx, c, cid, ctr.Files); err != nil {
		// Remove the container so we don't leave a broken half-started state.
		_ = c.ContainerRemove(ctx, cid, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("seeding config files: %w", err)
	}

	// ── Start container ──────────────────────────────────────────────────────
	fmt.Printf("Starting container %q...\n", containerName)
	if err := c.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		_ = c.ContainerRemove(ctx, cid, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("starting container %q: %w", containerName, err)
	}

	// ── Persist service record ───────────────────────────────────────────────
	if err := persistService(database, serviceID, name, scopeStr, cid, ctr.Image); err != nil {
		fmt.Fprintf(os.Stderr, "warning: persisting service state: %v\n", err)
	}

	fmt.Printf("  container id: %s\n", cid[:12])
	printPortAllocations(portResults)

	return cid, nil
}

// ---------------------------------------------------------------------------
// Image pulling
// ---------------------------------------------------------------------------

func ensureImage(c *client.Client, ref string) error {
	ctx, cancel := context.WithTimeout(context.Background(), containerOpTimeout)
	defer cancel()

	fmt.Printf("Pulling image %q (skipped if already present)...\n", ref)
	rc, err := c.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", ref, err)
	}
	defer rc.Close()
	// Consume the pull stream so progress is shown and the pull completes.
	_, _ = io.Copy(os.Stdout, rc)
	return nil
}

// ---------------------------------------------------------------------------
// File seeding
// ---------------------------------------------------------------------------

// seedFiles copies each declared config file into the container using the
// Docker API (PutArchive with an in-memory tar).
func seedFiles(ctx context.Context, c *client.Client, cid string, files []recipe.File) error {
	if len(files) == 0 {
		return nil
	}
	for _, f := range files {
		fmt.Printf("  seeding %s\n", f.Path)
		if err := putFile(ctx, c, cid, f); err != nil {
			return err
		}
	}
	return nil
}

// putFile uploads a single recipe.File into the container using a tar archive.
func putFile(ctx context.Context, c *client.Client, cid string, f recipe.File) error {
	mode := fileMode(f.Mode)
	tarBuf := buildTar(filepath.Base(f.Path), []byte(f.Content), mode)
	dir := filepath.ToSlash(filepath.Dir(f.Path))
	return c.CopyToContainer(ctx, cid, dir, tarBuf, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	})
}

// ---------------------------------------------------------------------------
// Port mapping helpers
// ---------------------------------------------------------------------------

func buildPortMappings(results []ports.Result) (dockernat.PortSet, dockernat.PortMap) {
	portSet := dockernat.PortSet{}
	portBindings := dockernat.PortMap{}

	for _, r := range results {
		proto := r.Protocol
		if proto == "" {
			proto = "tcp"
		}
		p := dockernat.Port(fmt.Sprintf("%d/%s", r.ContainerPort, proto))
		portSet[p] = struct{}{}
		portBindings[p] = []dockernat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: strconv.Itoa(r.AllocatedHostPort)},
		}
	}
	return portSet, portBindings
}

// ---------------------------------------------------------------------------
// State persistence
// ---------------------------------------------------------------------------

func persistService(database *sql.DB, serviceID, name, scope, containerID, img string) error {
	_, err := database.Exec(
		`INSERT INTO services (id, name, scope, container_id, image, status)
		 VALUES (?, ?, ?, ?, ?, 'running')
		 ON CONFLICT(id) DO UPDATE SET
		     container_id = excluded.container_id,
		     image        = excluded.image,
		     status       = 'running',
		     updated_at   = CURRENT_TIMESTAMP`,
		serviceID, name, scope, containerID, img,
	)
	if err != nil {
		return fmt.Errorf("persisting service %q: %w", name, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printOutputs(out recipe.Outputs) {
	if len(out.Env) == 0 && out.Instructions == "" {
		return
	}
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────")
	if len(out.Env) > 0 {
		fmt.Println("Environment variables:")
		for k, v := range out.Env {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	if out.Instructions != "" {
		fmt.Println()
		fmt.Println(out.Instructions)
	}
	fmt.Println("──────────────────────────────────────────────────────────")
}

func printPortAllocations(results []ports.Result) {
	for _, r := range results {
		fmt.Printf("  port %d → host:%d/%s\n", r.ContainerPort, r.AllocatedHostPort, r.Protocol)
	}
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func toSocketScope(s string) (socket.Scope, error) {
	switch s {
	case "system":
		return socket.ScopeSystem, nil
	case "project":
		return socket.ScopeProject, nil
	case "custom":
		return socket.ScopeCustom, nil
	default:
		return "", fmt.Errorf("unknown scope %q (must be system, project, or custom)", s)
	}
}

func mergeEnv(base map[string]string, overrides map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	return out
}

func parseEnvOverrides(pairs []string) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, _ := strings.Cut(p, "=")
		if k != "" {
			m[k] = v
		}
	}
	return m
}

// volumeName derives a stable Docker volume name for a container path.
// e.g. "datapower" + "/opt/ibm/datapower/drouter/config" →
// "lbmesh-datapower-opt-ibm-datapower-drouter-config"
func volumeName(serviceName, containerPath string) string {
	slug := strings.NewReplacer("/", "-", "\\", "-", ".", "-", "_", "-").Replace(containerPath)
	slug = strings.Trim(slug, "-")
	return "lbmesh-" + serviceName + "-" + slug
}
