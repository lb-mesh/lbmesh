package machine

import (
	"database/sql"
	"fmt"

	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// ServiceMemoryEntry records the resolved memory requirement for a single
// active service.
type ServiceMemoryEntry struct {
	// ServiceID is the UUID of the service row in SQLite.
	ServiceID string
	// Name is the service/recipe name.
	Name string
	// MinMemoryMB is the minimum memory reservation declared in the recipe.
	MinMemoryMB int
}

// ActiveFootprint returns the total minimum-memory footprint of all currently
// active (status = "running") services tracked in the database, by resolving
// each service name back to its recipe definition.
//
// Services whose recipe cannot be resolved are skipped (with their name noted
// in the returned skipped slice) rather than failing the whole calculation, so
// partial capacity information is still actionable.
func ActiveFootprint(db *sql.DB) (entries []ServiceMemoryEntry, totalMB int, skipped []string, err error) {
	rows, err := db.Query(
		`SELECT id, name FROM services WHERE status = 'running'`,
	)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("querying active services: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if scanErr := rows.Scan(&id, &name); scanErr != nil {
			return nil, 0, nil, fmt.Errorf("scanning service row: %w", scanErr)
		}

		resolved, resolveErr := recipe.Resolve(name)
		if resolveErr != nil {
			// Recipe not found or unreadable — record the name and continue so
			// the advisor can still warn about the services it does know about.
			skipped = append(skipped, name)
			continue
		}

		entry := ServiceMemoryEntry{
			ServiceID:   id,
			Name:        name,
			MinMemoryMB: resolved.Recipe.Resources.MinMemoryMB,
		}
		entries = append(entries, entry)
		totalMB += resolved.Recipe.Resources.MinMemoryMB
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, fmt.Errorf("iterating active services: %w", err)
	}

	return entries, totalMB, skipped, nil
}
