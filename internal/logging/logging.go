package logging

import (
	"log"
	"os"
)

var logger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)

func Get() *log.Logger {
	return logger
}
