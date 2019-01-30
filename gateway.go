// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"flag"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"

	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"

	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/peer"

	"fmt"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

var (
	mdcfg   = flag.String("mdcfg", "demo_fix_marketdata.cfg", "Market data FIX configuration file name")
	ordcfg  = flag.String("ordcfg", "demo_fix_orders.cfg", "Order flow FIX configuration file name")
	orders  = flag.Bool("orders", false, "enable order routing FIX endpoint")
	md      = flag.Bool("md", false, "enable market data FIX endpoint")
	ws      = flag.String("ws", "wss://api.bitfinex.com/ws/2", "v2 Websocket API URL")
	rst     = flag.String("rest", "https://api.bitfinex.com/v2/", "v2 REST API URL")
	sym     = flag.String("symbology", "", "symbol master, omit for passthrough symbology or provide a symbology master file")
	verbose = flag.Bool("v", false, "verbose logging")
	//flag.StringVar(&logfile, "logfile", "logs/debug.log", "path to the log file")
	//flag.StringVar(&configfile, "configfile", "config/server.cfg", "path to the config file")
)

var (
	//FIXConfigDirectory is the configuration directory for selecting order flow or market data options
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

	MarketData   *service.Service
	OrderRouting *service.Service

	factory peer.ClientFactory
}

// Start begins gateway operation
func (g *Gateway) Start() error {
	var err error
	if g.MarketData != nil {
		err = g.MarketData.Start()
		if err != nil {
			return err
		}
	}
	if g.OrderRouting != nil {
		err = g.OrderRouting.Start()
		if err != nil {
			return err
		}
	}
	return err
}

// Stop ceases gateway operation
func (g *Gateway) Stop() {
	if g.OrderRouting != nil {
		g.OrderRouting.Stop()
	}
	if g.MarketData != nil {
		g.MarketData.Stop()
	}
}

// New creates a gateway given the supplied settings
func New(mdSettings, orderSettings *quickfix.Settings, factory peer.ClientFactory, symbology symbol.Symbology) (*Gateway, error) {
	g := &Gateway{
		logger:  log.Logger,
		factory: factory,
	}
	var err error
	if mdSettings != nil {
		g.MarketData, err = service.New(factory, mdSettings, fix.MarketDataService, symbology)
		if err != nil {
			log.Logger.Fatal("create market data FIX", zap.Error(err))
			return nil, err
		}
	}
	if orderSettings != nil {
		g.OrderRouting, err = service.New(factory, orderSettings, fix.OrderRoutingService, symbology)
		if err != nil {
			log.Logger.Fatal("create order routing FIX", zap.Error(err))
			return nil, err
		}
	}
	return g, nil
}

// NonceFactory provides a simple interface for generating nonces
type NonceFactory interface {
	Create()
}

type defaultClientFactory struct {
	*websocket.Parameters
	RestURL string
	NonceFactory
}

func (d *defaultClientFactory) NewWs() *websocket.Client {
	if d.Parameters == nil {
		d.Parameters = websocket.NewDefaultParameters()
	}
	return websocket.NewWithParamsNonce(d.Parameters, peer.NewMultikeyNonceGenerator())
}

func (d *defaultClientFactory) NewRest() *rest.Client {
	if d.RestURL == "" {
		return rest.NewClient()
	}
	return rest.NewClientWithURLNonce(d.RestURL, peer.NewMultikeyNonceGenerator())
}

func main() {
	flag.Parse()
	var err error
	var mds, ords *quickfix.Settings
	var symbology symbol.Symbology
	if *sym == "" {
		log.Logger.Info("Symbology: passthrough")
		symbology = symbol.NewPassthroughSymbology()
	} else {
		log.Logger.Info(fmt.Sprintf("Symbology: %s", *sym))
		symbology, err = symbol.NewFileSymbology(*sym)
		if err != nil {
			log.Logger.Fatal("could not create file symbology", zap.Error(err))
		}
	}
	if *md {
		mdf, err := os.Open(path.Join(FIXConfigDirectory, *mdcfg))
		if err != nil {
			log.Logger.Fatal("FIX market data config", zap.Error(err))
		}
		mds, err = quickfix.ParseSettings(mdf)
		if err != nil {
			log.Logger.Fatal("parse FIX market data settings", zap.Error(err))
		}
	}
	if *orders {
		ordf, err := os.Open(path.Join(FIXConfigDirectory, *ordcfg))
		if err != nil {
			log.Logger.Fatal("FIX order flow config", zap.Error(err))
		}
		ords, err = quickfix.ParseSettings(ordf)
		if err != nil {
			log.Logger.Fatal("parse FIX order flow settings", zap.Error(err))
		}
	}
	params := websocket.NewDefaultParameters()
	params.URL = *ws
	params.LogTransport = *verbose
	factory := &defaultClientFactory{
		Parameters: params,
		RestURL:    *rst,
	}
	g, err := New(mds, ords, factory, symbology)
	if err != nil {
		log.Logger.Fatal("could not create gateway", zap.Error(err))
	}
	err = g.Start()
	if err != nil {
		log.Logger.Fatal("start FIX", zap.Error(err))
	}

	g.logger.Info("starting stat server")

	// TODO remove profiling below for deployments
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
