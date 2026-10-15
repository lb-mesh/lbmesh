package socket

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// networkOpTimeout is the maximum time allowed for a single network API call.
const networkOpTimeout = 10 * time.Second

// NetworkInfo holds the essential fields returned after creating or inspecting
// a managed lbmesh bridge network.
type NetworkInfo struct {
	// ID is the full Docker/Podman network ID.
	ID string

	// Name is the human-readable network name (e.g. "lbmesh-global").
	Name string

	// Driver is the network driver in use (always "bridge" for lbmesh networks).
	Driver string

	// Subnet is the IPv4 CIDR block assigned to the network, if reported by
	// the daemon. May be empty when the daemon assigns addresses dynamically.
	Subnet string
}

// EnsureNetwork creates a bridge network with the given name when it does not
// already exist, or returns the existing network's details when it does.
//
// The function is idempotent: calling it multiple times with the same name is
// safe and will not produce duplicate networks.
//
// Labels are added to every managed network so that lbmesh-owned networks can
// be distinguished from user-created ones during cleanup:
//
//	lbmesh.managed  = "true"
//	lbmesh.network  = <name>
func EnsureNetwork(c *client.Client, name string) (NetworkInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkOpTimeout)
	defer cancel()

	// Attempt to inspect first — avoids an extra create round-trip when the
	// network already exists (common during service restarts).
	resource, err := c.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		return networkInfoFromResource(resource), nil
	}

	// Only proceed to create when the error indicates "not found".
	if !errdefs.IsNotFound(err) {
		return NetworkInfo{}, fmt.Errorf("inspecting network %q: %w", name, err)
	}

	// Network does not exist — create it.
	resp, err := c.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:     "bridge",
		Attachable: true,
		Labels: map[string]string{
			"lbmesh.managed": "true",
			"lbmesh.network": name,
		},
	})
	if err != nil {
		return NetworkInfo{}, fmt.Errorf("creating network %q: %w", name, err)
	}

	// Fetch full details so we can return the subnet.
	resource, err = c.NetworkInspect(ctx, resp.ID, network.InspectOptions{})
	if err != nil {
		// Creation succeeded but inspect failed — return partial info.
		return NetworkInfo{ID: resp.ID, Name: name, Driver: "bridge"}, nil
	}

	return networkInfoFromResource(resource), nil
}

// ConnectContainer attaches a running container to the named network.
//
// This is called after a container has been started so that it joins the
// correct lbmesh bridge alongside other services in the same scope.
func ConnectContainer(c *client.Client, networkID, containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), networkOpTimeout)
	defer cancel()

	err := c.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{})
	if err != nil {
		// Tolerate "already connected" — idempotent by design.
		if errdefs.IsConflict(err) {
			return nil
		}
		return fmt.Errorf("connecting container %s to network %s: %w", containerID, networkID, err)
	}
	return nil
}

// RemoveNetwork deletes a bridge network by ID or name.
//
// The function is a no-op (returns nil) when the network no longer exists,
// making it safe to call during cleanup without checking first.
func RemoveNetwork(c *client.Client, networkID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), networkOpTimeout)
	defer cancel()

	err := c.NetworkRemove(ctx, networkID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("removing network %s: %w", networkID, err)
	}
	return nil
}

// ActiveContainerCount returns the number of containers currently connected to
// the given network. Returns 0 when the network does not exist.
//
// Callers use this to decide whether a project-scoped or custom-scoped network
// is safe to remove during teardown.
func ActiveContainerCount(c *client.Client, networkID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkOpTimeout)
	defer cancel()

	resource, err := c.NetworkInspect(ctx, networkID, network.InspectOptions{Verbose: false})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("inspecting network %s: %w", networkID, err)
	}
	return len(resource.Containers), nil
}

// networkInfoFromResource maps a network.Summary (the type returned by the
// Docker SDK's NetworkInspect) into the leaner NetworkInfo struct.
func networkInfoFromResource(r network.Inspect) NetworkInfo {
	info := NetworkInfo{
		ID:     r.ID,
		Name:   r.Name,
		Driver: r.Driver,
	}
	if len(r.IPAM.Config) > 0 {
		info.Subnet = r.IPAM.Config[0].Subnet
	}
	return info
}
