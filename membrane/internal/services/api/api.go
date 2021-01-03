package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jhead/phantom/membrane/internal/services/model"
	"github.com/jhead/phantom/membrane/internal/services/servers"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"

	"github.com/gofiber/fiber"
)

// Service provides an HTTP server and JSON REST API for managing phantom.
// Does not interact with the DB directly, but through the server management service.
type Service struct {
	opt     model.Settings
	servers servers.ServerManagement
	app     *fiber.App
}

// Wraps the request context so we can add some functions to it
type requestContext struct {
	*fiber.Ctx
}

// A generic JSON object for an API error message
type errorResponse struct {
	Error string `json:"error"`
}

var genericError = errors.Errorf("An unexpected error occurred")

// New creates a new API service that provides an HTTP server and JSON REST API for managing phantom
func New(opt model.Settings, servers servers.ServerManagement) Service {
	log.Info().Msg("Starting up API server")

	app := fiber.New()
	app.Settings.DisableStartupMessage = true

	api := Service{opt, servers, app}

	registerRoute(app.Get, "/api", api.helloEndpoint)
	registerRoute(app.Get, "/api/servers", api.listServersEndpoint)
	registerRoute(app.Get, "/api/servers/:id", api.getServerEndpoint)
	registerRoute(app.Put, "/api/servers/:id", api.createServerEndpoint)
	registerRoute(app.Put, "/api/servers/:id/start", api.startServerEndpoint)
	registerRoute(app.Put, "/api/servers/:id/stop", api.stopServerEndpoint)
	registerRoute(app.Delete, "/api/servers/:id", api.deleteServerEndpoint)

	return api
}

func (api Service) Start() error {
	localURL := fmt.Sprintf("http://localhost:%d/api", api.opt.ApiBindPort)

	// Create a channel that will be notified when the HTTP server is up
	cancel := logWhenLive(localURL)
	defer cancel()

	return api.app.Listen(int(api.opt.ApiBindPort))
}

func (api Service) Close() error {
	return api.app.Shutdown()
}

// Wraps fiber's route.Get, route.Post, etc.
type registerRouteFunc func(string, ...func(*fiber.Ctx)) *fiber.App

// Wraps fiber's route registration for a path and handler and logs it
func registerRoute(doRegister registerRouteFunc, path string, handler func(requestContext)) {
	log.Debug().Msgf("Registering route: %s", path)
	doRegister(path, func(ctx *fiber.Ctx) {
		handler(requestContext{ctx})
	})
}

func (ctx requestContext) jsonError(code int, err error) {
	ctx.Status(code)
	ctx.JSON(errorResponse{err.Error()})
}

func (ctx requestContext) jsonUnexpectedError(err error) {
	log.Error().Err(err).Msg("An unexpected error occurred")
	ctx.jsonError(500, genericError)
}

// Logs a message when the HTTP server starts up
func logWhenLive(url string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	// Check ever 250ms
	timer := time.NewTimer(250 * time.Millisecond)

	// Checks if the HTTP server is up. If it is, it cancels the context.
	check := func() {
		if _, err := http.Get(url); err == nil {
			log.Info().Msgf("API is live! %s", url)
			cancel()
		}
	}

	// Loop and check asynchronously
	go func() {
		for {
			// Waits for either the timer or a cancellation
			select {
			case <-ctx.Done():
				// Listener failed or server is up!
				return
			case <-timer.C:
				// Time to check again!
				check()
			}
		}
	}()

	// Return the cancel func so that we can cancel early if the listener fails
	return cancel
}

func (api Service) helloEndpoint(ctx requestContext) {
	ctx.SendString("hiya!")
}
