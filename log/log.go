package log

import (
	"log"
	"os"

	"go.uber.org/zap"
)

// Logger is a global instance used for logging
var Logger *zap.Logger

func init() {
	if os.Getenv("DEBUG") == "1" {
		logger, err := zap.NewDevelopment()
		if err != nil {
			log.Fatalf("failed to initialize logger: %s", err)
		}
		Logger = logger
	} else {
		logger, err := zap.NewProduction()
		if err != nil {
			log.Fatalf("failed to initialize logger: %s", err)
		}
		Logger = logger
	}
}
