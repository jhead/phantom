package model

import (
	"github.com/jhead/phantom/internal/proxy"
	"github.com/pkg/errors"
)

var ServerExistsError = errors.Errorf("Server already exists")
var ServerNotFoundError = errors.Errorf("Server does not exist")

type Server struct {
	ID    string           `json:"-"`
	Name  string           `json:"name"`
	Prefs proxy.ProxyPrefs `json:"prefs"`
}

type Settings struct {
	ApiBindPort uint16 `json:"apiPort"`
}

func DefaultSettings() Settings {
	return Settings{
		ApiBindPort: 7377,
	}
}
