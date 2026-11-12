package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
)

type containerLister interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

type instanceRow struct {
	ContainerID string
	Name        string
	Scope       string
	Status      string
	Ports       string
	Image       string
	IsRunning   bool
}

func newStatusCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display all running instances, scopes, container IDs, and port mappings",
		Long: `status queries the local state database and the active container engine
to print a live ASCII table of running projects, their scopes, container IDs,
allocated host ports, and basic resource metrics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			database, err := db.OpenDefaultAndMigrate()
			if err != nil {
				return fmt.Errorf("opening state database: %w", err)
			}
			defer database.Close()

			c, _, err := socket.Detect()
			if err != nil {
				return err
			}
			defer c.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			rows, err := collectInstances(ctx, database, c)
			if err != nil {
				return err
			}

			return renderStatus(cmd.OutOrStdout(), rows, all)
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, "Show all instances (default shows running only)")

	return cmd
}

func collectInstances(ctx context.Context, database *sql.DB, cl containerLister) ([]instanceRow, error) {
	type dbService struct {
		id          string
		projectID   string
		name        string
		scope       string
		containerID string
		image       string
		status      string
	}

	dbServicesByCID := make(map[string]dbService)
	portsByServiceID := make(map[string][]string)

	if database != nil {
		sRows, err := database.QueryContext(ctx,
			`SELECT id, COALESCE(project_id, ''), name, scope, container_id, image, status FROM services`)
		if err == nil {
			defer sRows.Close()
			for sRows.Next() {
				var s dbService
				if err := sRows.Scan(&s.id, &s.projectID, &s.name, &s.scope, &s.containerID, &s.image, &s.status); err == nil {
					if s.containerID != "" {
						dbServicesByCID[s.containerID] = s
						if len(s.containerID) >= 12 {
							dbServicesByCID[s.containerID[:12]] = s
						}
					}
				}
			}
		}

		pRows, err := database.QueryContext(ctx,
			`SELECT service_id, container_port, host_port, protocol FROM port_allocations ORDER BY host_port`)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var serviceID, proto string
				var cPort, hPort int
				if err := pRows.Scan(&serviceID, &cPort, &hPort, &proto); err == nil {
					portsByServiceID[serviceID] = append(portsByServiceID[serviceID],
						fmt.Sprintf("127.0.0.1:%d->%d/%s", hPort, cPort, proto))
				}
			}
		}
	}

	containers, err := cl.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var rows []instanceRow

	for _, ctr := range containers {
		isManaged := ctr.Labels["lbmesh.managed"] == "true" ||
			ctr.Labels["lbmesh.recipe"] != "" ||
			ctr.Labels["lbmesh.service"] != ""

		var rawName string
		for _, n := range ctr.Names {
			trimmed := strings.TrimPrefix(n, "/")
			if strings.HasPrefix(trimmed, "lbmesh-") {
				isManaged = true
				rawName = trimmed
				break
			}
		}

		var dbSvc dbService
		var foundInDB bool
		if svc, ok := dbServicesByCID[ctr.ID]; ok {
			isManaged = true
			dbSvc = svc
			foundInDB = true
		} else if len(ctr.ID) >= 12 {
			if svc, ok := dbServicesByCID[ctr.ID[:12]]; ok {
				isManaged = true
				dbSvc = svc
				foundInDB = true
			}
		}

		if !isManaged {
			continue
		}

		name := ""
		if foundInDB && dbSvc.name != "" {
			name = dbSvc.name
		} else if r, ok := ctr.Labels["lbmesh.recipe"]; ok && r != "" {
			name = r
		} else if rawName != "" {
			name = strings.TrimPrefix(rawName, "lbmesh-")
		} else if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(strings.TrimPrefix(ctr.Names[0], "/"), "lbmesh-")
		}

		scope := "project"
		if foundInDB && dbSvc.scope != "" {
			scope = dbSvc.scope
		} else if s, ok := ctr.Labels["lbmesh.scope"]; ok && s != "" {
			scope = s
		}

		ports := formatContainerPorts(ctr.Ports)
		if (ports == "-" || ports == "") && foundInDB {
			if pList, ok := portsByServiceID[dbSvc.id]; ok && len(pList) > 0 {
				ports = strings.Join(pList, ", ")
			}
		}
		if ports == "" {
			ports = "-"
		}

		shortID := ctr.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		status := ctr.State
		if status == "" {
			status = ctr.Status
		}
		isRunning := ctr.State == "running" || strings.HasPrefix(strings.ToLower(ctr.Status), "up")

		rows = append(rows, instanceRow{
			ContainerID: shortID,
			Name:        name,
			Scope:       scope,
			Status:      status,
			Ports:       ports,
			Image:       ctr.Image,
			IsRunning:   isRunning,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ContainerID < rows[j].ContainerID
	})

	return rows, nil
}

func formatContainerPorts(ports []container.Port) string {
	if len(ports) == 0 {
		return "-"
	}
	var parts []string
	for _, p := range ports {
		if p.PublicPort > 0 {
			if p.IP != "" {
				parts = append(parts, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			} else {
				parts = append(parts, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
			}
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(parts, ", ")
}

func renderStatus(out io.Writer, rows []instanceRow, all bool) error {
	var filtered []instanceRow
	for _, r := range rows {
		if all || r.IsRunning {
			filtered = append(filtered, r)
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintln(out, "status: no running instances found.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tNAME\tSCOPE\tSTATUS\tPORTS\tIMAGE")
	for _, r := range filtered {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ContainerID, r.Name, r.Scope, r.Status, r.Ports, r.Image)
	}
	return w.Flush()
}
