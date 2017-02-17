// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"path/filepath"

	"github.com/bitfinexcom/bfxfixgw/link"
	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/quickfixgo/quickfix"
	"github.com/uber-go/zap"
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
	logger zap.Logger
	links  []*link.Link
}

func main() {
	ps, err := filepath.Glob(path.Join(FIXConfigDirectory, "server", "*.cfg"))
	if err != nil {
		log.Logger.Fatal("glob FIX configs", zap.Error(err))
	}

	links := []*link.Link{}
	for _, p := range ps {
		cfg, err := os.Open(p)
		if err != nil {
			log.Logger.Fatal("open FIX settings", zap.Error(err))
		}
		s, err := quickfix.ParseSettings(cfg)
		if err != nil {
			log.Logger.Fatal("parse FIX settings", zap.Error(err))
		}

		l, err := link.NewLink(s)
		if err != nil {
			log.Logger.Fatal("creating new Link", zap.Error(err))
		}
		err = l.Establish()
		if err != nil {
			log.Logger.Fatal("establish new Link", zap.Error(err))
		}
		links = append(links, l)
	}

	g := &Gateway{logger: log.Logger, links: links}

	g.logger.Info("starting stat server")
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
