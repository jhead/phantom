package jsondb

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/jhead/phantom/membrane/internal/services/db"
	"github.com/jhead/phantom/membrane/internal/services/model"
	"github.com/rs/zerolog/log"
)

type Database struct {
	persist db.Persistence
	mutex   *sync.RWMutex
}

type data struct {
	Servers  map[string]model.Server `json:"servers"`
	Settings model.Settings          `json:"settings"`
}

func New(persist db.Persistence) (Database, error) {
	return Database{
		persist,
		&sync.RWMutex{},
	}, nil
}

func (database Database) writeJSON(contents data) error {
	bytes, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return err
	}

	jsonString := string(bytes)

	// Obtain a lock before writing to file
	database.mutex.Lock()
	defer database.mutex.Unlock()

	if err = database.persist.StoreData(jsonString); err != nil {
		return err
	}

	log.Debug().Msgf("Wrote database file: %v", jsonString)
	return nil
}

func (database Database) readJSON() (data, error) {
	// Read file w/ a lock to avoid concurrent access issues
	database.mutex.RLock()
	jsonString, err := database.persist.ReadData()
	database.mutex.RUnlock()

	if err != nil {
		// Ignore error if the file doesn't exist
		if !os.IsNotExist(err) {
			return data{}, err
		}
	}

	log.Debug().Msgf("Read database file: %v", jsonString)

	// Create a new DB
	if len(jsonString) == 0 {
		log.Info().Msg("No existing database found")
		return database.createNewDatabase()
	}

	// Read an existing DB
	contents := emptyData()
	if err := json.Unmarshal([]byte(jsonString), &contents); err != nil {
		log.Warn().Msgf("JSON parsing error while reading database file: %v", err)
		// Probably a syntax error or something, not much we can do
		// return database.replaceBrokenDatabase()
	}

	// Successfuly read existing DB
	return contents, nil
}

// func (database Database) replaceBrokenDatabase() (data, error) {
// 	// Rename broken file as a backup and create an empty one
// 	timestamp := time.Now().Unix()

// 	backupPath := fmt.Sprintf(
// 		"%s-backup-%d",
// 		path.Base(database.path),
// 		timestamp,
// 	)

// 	if err := os.Rename(database.path, backupPath); err != nil {
// 		log.Error().Msgf("Failed to backup broken database file: %v", err)
// 	} else {
// 		log.Info().Msgf("Moved the broken database file to %s", backupPath)
// 	}

// 	return database.createNewDatabase()
// }

func (database Database) createNewDatabase() (data, error) {
	log.Info().Msg("Creating a new database")
	contents := emptyData()
	return contents, database.writeJSON(contents)
}

func emptyData() data {
	return data{
		make(map[string]model.Server),
		model.DefaultSettings(),
	}
}
