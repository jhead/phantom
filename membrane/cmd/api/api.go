package membrane

import (
	"fmt"

	"github.com/jhead/phantom/membrane/internal/services/api"
	"github.com/jhead/phantom/membrane/internal/services/db/jsondb"
	"github.com/jhead/phantom/membrane/internal/services/servers"
	"github.com/rs/zerolog/log"
)

func Start() {
	fmt.Println("Starting up in API server mode")

	database, err := jsondb.New("./phantom.json")
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to open database")
		return
	}

	settings, err := database.GetSettings()
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load settings")
		return
	}

	apiService := api.New(settings, servers.New(database))

	if err := apiService.Start(); err != nil {
		log.Fatal().Msgf("Failed to start API server: %v", err)
	}
}
