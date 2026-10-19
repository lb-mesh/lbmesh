package ports_test

import (
	"database/sql"
	"fmt"
	"net"
	"testing"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/ports"
)

// openTestDB opens an in-memory SQLite database and runs migrations.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenAndMigrate(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestAllocate_PreferredPortFree verifies that when the preferred port is
// completely free the broker returns it unchanged.
func TestAllocate_PreferredPortFree(t *testing.T) {
	database := openTestDB(t)
	// Insert a dummy service row so the FK constraint on port_allocations is satisfied.
	_, err := database.Exec(
		`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
		"svc-1", "test", "system", "", "", "created",
	)
	if err != nil {
		t.Fatalf("inserting service: %v", err)
	}

	broker := ports.New(database)
	result, err := broker.Allocate(ports.Request{
		ServiceID:         "svc-1",
		ContainerPort:     5432,
		PreferredHostPort: 15432,
		Protocol:          "tcp",
		EnvKey:            "POSTGRES_HOST_PORT",
	})
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if result.AllocatedHostPort != 15432 {
		t.Errorf("expected 15432, got %d", result.AllocatedHostPort)
	}
	if result.EnvInjections["POSTGRES_HOST_PORT"] != "15432" {
		t.Errorf("unexpected env injection: %v", result.EnvInjections)
	}
}

// TestAllocate_SkipsDBConflict verifies that a port already recorded in
// port_allocations is skipped and the next free port is returned.
func TestAllocate_SkipsDBConflict(t *testing.T) {
	database := openTestDB(t)

	// Two services: svc-1 already owns port 15000, svc-2 wants the same.
	for _, id := range []string{"svc-1", "svc-2"} {
		if _, err := database.Exec(
			`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
			id, "test-"+id, "system", "", "", "created",
		); err != nil {
			t.Fatalf("inserting service %s: %v", id, err)
		}
	}

	// Pre-occupy port 15000 for svc-1.
	if _, err := database.Exec(
		`INSERT INTO port_allocations (service_id, container_port, host_port, protocol) VALUES (?,?,?,?)`,
		"svc-1", 5432, 15000, "tcp",
	); err != nil {
		t.Fatalf("pre-inserting allocation: %v", err)
	}

	broker := ports.New(database)
	result, err := broker.Allocate(ports.Request{
		ServiceID:         "svc-2",
		ContainerPort:     5432,
		PreferredHostPort: 15000,
		Protocol:          "tcp",
	})
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if result.AllocatedHostPort == 15000 {
		t.Error("broker returned a port already recorded in SQLite")
	}
	if result.AllocatedHostPort < 15001 {
		t.Errorf("expected port >= 15001, got %d", result.AllocatedHostPort)
	}
}

// TestAllocate_SkipsPhysicallyBoundPort verifies that a port bound by a real
// OS listener is also skipped.
func TestAllocate_SkipsPhysicallyBoundPort(t *testing.T) {
	// Start an actual listener to occupy a port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not start test listener: %v", err)
	}
	defer ln.Close()
	boundPort := ln.Addr().(*net.TCPAddr).Port

	database := openTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
		"svc-3", "test", "system", "", "", "created",
	); err != nil {
		t.Fatalf("inserting service: %v", err)
	}

	broker := ports.New(database)
	result, err := broker.Allocate(ports.Request{
		ServiceID:         "svc-3",
		ContainerPort:     3306,
		PreferredHostPort: boundPort,
		Protocol:          "tcp",
	})
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if result.AllocatedHostPort == boundPort {
		t.Errorf("broker returned the bound port %d — TCP probe failed to detect it", boundPort)
	}
}

// TestAllocate_NoEnvKey verifies that an empty EnvKey produces an empty
// injections map (not nil).
func TestAllocate_NoEnvKey(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
		"svc-4", "test", "system", "", "", "created",
	); err != nil {
		t.Fatalf("inserting service: %v", err)
	}

	broker := ports.New(database)
	result, err := broker.Allocate(ports.Request{
		ServiceID:         "svc-4",
		ContainerPort:     6379,
		PreferredHostPort: 16379,
		Protocol:          "tcp",
		// EnvKey intentionally omitted.
	})
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if len(result.EnvInjections) != 0 {
		t.Errorf("expected empty EnvInjections, got %v", result.EnvInjections)
	}
}

// TestAllocateMany verifies that multiple requests are resolved in order and
// each receives a distinct host port.
func TestAllocateMany(t *testing.T) {
	database := openTestDB(t)
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("svc-%d", i)
		if _, err := database.Exec(
			`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
			id, "test-"+id, "system", "", "", "created",
		); err != nil {
			t.Fatalf("inserting service %s: %v", id, err)
		}
	}

	broker := ports.New(database)
	results, err := broker.AllocateMany([]ports.Request{
		{ServiceID: "svc-1", ContainerPort: 5432, PreferredHostPort: 15100, Protocol: "tcp", EnvKey: "PG_PORT"},
		{ServiceID: "svc-2", ContainerPort: 6379, PreferredHostPort: 15100, Protocol: "tcp", EnvKey: "REDIS_PORT"},
		{ServiceID: "svc-3", ContainerPort: 3306, PreferredHostPort: 15100, Protocol: "tcp", EnvKey: "MYSQL_PORT"},
	})
	if err != nil {
		t.Fatalf("AllocateMany: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	seen := make(map[int]bool)
	for _, r := range results {
		if seen[r.AllocatedHostPort] {
			t.Errorf("duplicate allocated host port %d", r.AllocatedHostPort)
		}
		seen[r.AllocatedHostPort] = true
	}
}

// TestAllocate_UDP verifies that UDP ports can be allocated; the probe used to
// call net.Listen("udp"), which always errors, so every UDP port looked taken.
func TestAllocate_UDP(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO services (id, name, scope, container_id, image, status) VALUES (?,?,?,?,?,?)`,
		"svc-udp", "dns", "project", "", "coredns", "created",
	); err != nil {
		t.Fatalf("inserting service: %v", err)
	}

	res, err := ports.New(database).Allocate(ports.Request{
		ServiceID: "svc-udp", ContainerPort: 53, PreferredHostPort: 25353, Protocol: "udp",
	})
	if err != nil {
		t.Fatalf("Allocate udp: %v", err)
	}
	if res.Protocol != "udp" || res.AllocatedHostPort < 25353 {
		t.Fatalf("unexpected result %+v", res)
	}
}

// TestAllocate_UnknownServiceErrors verifies that a failing insert (here a
// foreign-key violation) is reported instead of being retried as a port clash.
func TestAllocate_UnknownServiceErrors(t *testing.T) {
	database := openTestDB(t)
	_, err := ports.New(database).Allocate(ports.Request{
		ServiceID: "missing", ContainerPort: 80, PreferredHostPort: 25400,
	})
	if err == nil {
		t.Fatal("expected an error for a service that does not exist")
	}
}
