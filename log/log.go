package log

import (
	"os"

	"github.com/uber-go/zap"
)

var Logger zap.Logger

func init() {
	if os.Getenv("DEBUG") == "1" {
		Logger = zap.New(zap.NewJSONEncoder(), zap.DebugLevel)
	} else {
		Logger = zap.New(zap.NewJSONEncoder())
	}
}
