package db

import (
	"time"
)

// Project represents an orchestrator project workspace.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Network   string    `json:"network"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service represents a managed container service instance.
type Service struct {
	ID          string    `json:"id"`
	ProjectID   *string   `json:"project_id,omitempty"`
	Name        string    `json:"name"`
	Scope       string    `json:"scope"` // "system", "project", "custom"
	ContainerID string    `json:"container_id"`
	Image       string    `json:"image"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PortAllocation represents an allocated host-to-container port mapping.
type PortAllocation struct {
	ID            int64     `json:"id"`
	ServiceID     string    `json:"service_id"`
	ContainerPort int       `json:"container_port"`
	HostPort      int       `json:"host_port"`
	Protocol      string    `json:"protocol"` // "tcp", "udp"
	AllocatedAt   time.Time `json:"allocated_at"`
}
