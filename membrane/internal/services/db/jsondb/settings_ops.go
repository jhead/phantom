package jsondb

import (
	"github.com/jhead/phantom/membrane/internal/services/model"
	"github.com/rs/zerolog/log"
)

func (database Database) GetSettings() (model.Settings, error) {
	contents, err := database.readJSON()

	empty := model.Settings{}
	if contents.Settings == empty {
		log.Warn().Msg("Couldn't find settings in database, using defaults")
		contents.Settings = model.DefaultSettings()
		database.writeJSON(contents)
	}

	return contents.Settings, err
}
