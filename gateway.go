// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"

	"github.com/bitfinexcom/bfxfixgw/fix"
	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

var (
	FIXConfigDirectory = configDirectory()
)

func configDirectory() string {
	d := os.Getenv("FIX_SETTINGS_DIRECTORY")
	if d == "" {
		return "./config"
	}
	return d
}

// Gateway is a tunnel that enables a FIX client to talk to the bitfinex websocket API
// and vice versa.
type Gateway struct {
	logger *zap.Logger
	*fix.FIX
}

func (g *Gateway) Start() error {
	return g.FIX.Up()
}

func main() {
	f, err := os.Open(path.Join(FIXConfigDirectory, "bfx.cfg"))
	if err != nil {
		log.Logger.Fatal("FIX config", zap.Error(err))
	}
	s, err := quickfix.ParseSettings(f)
	if err != nil {
		log.Logger.Fatal("parse FIX settings", zap.Error(err))
	}
	fix, err := fix.NewFIX(s)
	if err != nil {
		log.Logger.Fatal("create FIX", zap.Error(err))
	}
	g := &Gateway{logger: log.Logger, FIX: fix}
	err = g.Start()
	if err != nil {
		log.Logger.Fatal("start FIX", zap.Error(err))
	}

	g.logger.Info("starting stat server")
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
