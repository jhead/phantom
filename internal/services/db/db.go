package db

import "github.com/jhead/phantom/internal/services/model"

type Database interface {
	// Server ops
	ListServers() (map[string]model.Server, error)
	GetServer(id string) (model.Server, error)
	CreateServer(server model.Server) error
	UpdateServer(server model.Server) error
	DeleteServer(id string) error

	// Settings
	GetSettings() (model.Settings, error)
}
