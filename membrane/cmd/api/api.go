package membrane

import (
	"fmt"

	"github.com/jhead/phantom/membrane/internal/services/api"
	"github.com/jhead/phantom/membrane/internal/services/db"
	"github.com/jhead/phantom/membrane/internal/services/db/jsondb"
	"github.com/jhead/phantom/membrane/internal/services/servers"
	"github.com/rs/zerolog/log"
)

// NativePersistence provides a way to store/retrieve data in
// native platform-specific code.
type NativePersistence interface {
	db.Persistence
}

func Start(persist NativePersistence) {
	fmt.Println("Starting up in API server mode")

	database, err := jsondb.New(persist)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to open database")
		return
	}

	settings, err := database.GetSettings()
	if err != nil {
		log.Error().Err(err).Msgf("Failed to load settings")
		return
	}

	apiService := api.New(settings, servers.New(database))

	if err := apiService.Start(); err != nil {
		log.Error().Msgf("Failed to start API server: %v", err)
	}
}
