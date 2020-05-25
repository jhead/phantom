package servers

import (
	"context"

	"github.com/jhead/phantom/internal/exec"
	"github.com/jhead/phantom/internal/proxy"
	"github.com/jhead/phantom/internal/services/db"
	"github.com/jhead/phantom/internal/services/model"
	"github.com/rs/zerolog/log"
)

// ServerManagement provides a way to start, stop, and manage phantom proxies for game servers
type ServerManagement interface {
	// Model ops
	List() (map[string]model.Server, error)
	Create(model.Server) error
	Get(id string) (model.Server, error)
	// Update(model.Server) error
	Delete(id string) error

	// Proxy ops
	Start(id string) error
	Stop(id string) error
}

// Service is the implementation of the ServerManagement service
type Service struct {
	db           db.Database
	ex           exec.Executor
	proxyService proxyService
}

// Container to hold the proxyMap without passing a reference to the whole Service to another goroutine.
// Keeps it a little safer. Actions contain a pointer to this so that they can access/update the map inside.
type proxyService struct {
	proxies proxyMap
}

type proxyMap map[string]*proxy.ProxyServer

type actionStartServer struct {
	proxyService *proxyService
	server       model.Server
}

type actionStopServer struct {
	proxyService *proxyService
	id           string
}

// New creates a new service for managing servers
func New(db db.Database) Service {
	log.Info().Msg("Starting up server management service")

	service := Service{
		db,
		// Single thread because we have concurrent map access
		exec.NewSingleThread(),
		proxyService{
			make(proxyMap),
		},
	}

	go service.logStats()

	return service
}

func (service Service) List() (map[string]model.Server, error) {
	return service.db.ListServers()
}

func (service Service) Get(id string) (model.Server, error) {
	return service.db.GetServer(id)
}

// Create saves the provided server to the DB, if it doesn't exist already.
func (service Service) Create(server model.Server) error {
	// Ensure that server doesn't exist already
	_, err := service.db.GetServer(server.ID)

	switch err {
	case model.ServerNotFoundError:
	case nil:
		return model.ServerExistsError
	default:
		return err
	}

	if err = service.db.CreateServer(server); err != nil {
		return err
	}

	return nil
}

// Start will asynchronously start a phantom proxy for the server associated with the provided ID
func (service Service) Start(id string) error {
	return service.existingServerAction(id, func(server model.Server) exec.Action {
		return actionStartServer{&service.proxyService, server}
	})
}

// Stop will asynchronously stop a running phantom proxy for the server associated with the provided ID
func (service Service) Stop(id string) error {
	return service.existingServerAction(id, func(server model.Server) exec.Action {
		return actionStopServer{&service.proxyService, server.ID}
	})
}

// Delete will asynchronously stop a phantom proxy for the server and delete it from the DB
func (service Service) Delete(id string) error {
	if err := service.Stop(id); err != nil {
		return err
	}

	// todo: ^^^^^^^^ is async, this is wrong
	err := service.db.DeleteServer(id)

	switch err {
	case model.ServerNotFoundError:
		// Already deleted
	case nil:
		// Success
	default:
		return err
	}

	return nil
}

func (service Service) existingServerAction(id string, buildAction func(server model.Server) exec.Action) error {
	server, err := service.db.GetServer(id)

	if err != nil {
		return err
	}

	service.ex.Execute(context.TODO(), buildAction(server))

	return nil
}

func (service Service) logStats() {
	servers, err := service.db.ListServers()

	if err != nil {
		log.Error().Msgf("Failed to list servers: %v", err)
	}

	log.Info().Msgf("Loaded %d servers from DB", len(servers))
	for _, server := range servers {
		log.Info().Msgf(" - %s", server.ID)
	}
}

/** Actions **/

func (cmd actionStartServer) Execute(ctx context.Context) error {
	if _, exists := cmd.proxyService.proxies[cmd.server.ID]; exists {
		log.Info().Msgf("Server %s already running!", cmd.server.ID)
		return nil
	}

	proxy, err := proxy.New(cmd.server.Prefs)
	if err != nil {
		return err
	}

	cmd.proxyService.proxies[cmd.server.ID] = proxy
	go proxy.Start()

	return nil
}

func (cmd actionStopServer) Execute(ctx context.Context) error {
	proxy, exists := cmd.proxyService.proxies[cmd.id]

	if !exists {
		log.Info().Msgf("Cannot stop server %s, it isn't running!", cmd.id)
		return nil
	}

	proxy.Close()
	delete(cmd.proxyService.proxies, cmd.id)

	return nil
}
