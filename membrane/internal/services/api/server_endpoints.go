package api

import (
	"github.com/jhead/phantom/membrane/internal/services/model"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

func (api Service) listServersEndpoint(ctx requestContext) {
	servers, err := api.servers.List()

	if err != nil {
		ctx.jsonUnexpectedError(err)
		return
	}

	ctx.JSON(servers)
}

func (api Service) getServerEndpoint(ctx requestContext) {
	id := *ctx.getParamID()

	server, err := api.servers.Get(id)

	if validateExistingServerOp(ctx, err) {
		return
	}

	ctx.JSON(server)
}

func (api Service) createServerEndpoint(ctx requestContext) {
	id := *ctx.getParamID()

	server := &model.Server{
		ID: id,
	}

	// Read server object from request body JSON
	if err := ctx.BodyParser(server); err != nil {
		log.Warn().Err(err).Msgf("Received invalid server object")
		ctx.jsonError(400, errors.Errorf("Received invalid server object"))
		return
	}

	// Try to create a server
	if err := api.servers.Create(*server); err != nil {
		// Map errors to status codes and respond with the error message
		switch err {
		case model.ServerExistsError:
			ctx.jsonError(400, err)
		default:
			ctx.jsonUnexpectedError(err)
		}
		return
	}

	ctx.Status(201)
	log.Info().Msgf("Created server: %s", id)
}

func (api Service) startServerEndpoint(ctx requestContext) {
	id := *ctx.getParamID()

	err := api.servers.Start(id)
	if validateExistingServerOp(ctx, err) {
		return
	}

	log.Info().Msgf("Starting server: %s", id)
}

func (api Service) stopServerEndpoint(ctx requestContext) {
	id := *ctx.getParamID()

	err := api.servers.Stop(id)
	if validateExistingServerOp(ctx, err) {
		return
	}

	log.Info().Msgf("Stopping server: %s", id)
}

func (api Service) deleteServerEndpoint(ctx requestContext) {
	id := *ctx.getParamID()

	err := api.servers.Delete(id)
	if validateExistingServerOp(ctx, err) {
		return
	}

	log.Info().Msgf("Deleting server: %s", id)
}

// Validates an error from an operation that depends on an existing server and accepts the server ID.
// Returns true if the error was matched and handled, false otherwise to continue handling.
func validateExistingServerOp(ctx requestContext, err error) bool {
	if err != nil {
		switch err {
		case model.ServerNotFoundError:
			ctx.jsonError(404, err)
		default:
			ctx.jsonUnexpectedError(err)
		}
		return true
	}

	return false
}

func (ctx requestContext) getParamID() *string {
	id := ctx.Params("id")

	if id == "" {
		return nil
	}

	return &id
}
