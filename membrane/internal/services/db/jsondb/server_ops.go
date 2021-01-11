package jsondb

import "github.com/jhead/phantom/membrane/internal/services/model"

// Functions for manipulating the model.Server model in the JSON database

func (database Database) ListServers() (map[string]model.Server, error) {
	contents, err := database.readJSON()

	if err != nil {
		return nil, nil
	}

	updatedServers := make(map[string]model.Server, len(contents.Servers))
	for id, server := range contents.Servers {
		// ID isn't stored in the object since it's already the map key
		server.ID = id
		updatedServers[id] = server
	}

	return updatedServers, nil
}

func (database Database) GetServer(id string) (model.Server, error) {
	servers, err := database.ListServers()

	if err != nil {
		return model.Server{}, err
	}

	if server, exists := servers[id]; !exists {
		return model.Server{}, model.ServerNotFoundError
	} else {
		return server, nil
	}
}

func (database Database) CreateServer(server model.Server) error {
	contents, err := database.readJSON()
	if err != nil {
		return err
	}

	// Init map if null
	if contents.Servers == nil {
		contents.Servers = make(map[string]model.Server)
	} else if _, exists := contents.Servers[server.ID]; exists {
		// Check if server exists
		return model.ServerExistsError
	}

	contents.Servers[server.ID] = server

	return database.writeJSON(contents)
}

func (database Database) UpdateServer(server model.Server) error {
	return nil
}

func (database Database) DeleteServer(id string) error {
	contents, err := database.readJSON()
	if err != nil {
		return err
	}

	if _, exists := contents.Servers[id]; !exists {
		return model.ServerNotFoundError
	}

	delete(contents.Servers, id)

	return database.writeJSON(contents)
}
