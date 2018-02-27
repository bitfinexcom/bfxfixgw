// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"sync"

	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"

	"github.com/bitfinexcom/bfxfixgw/fix"
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service"

	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

var (
	mdcfg  = flag.String("mdcfg", "demo_fix_marketdata.cfg", "Market data FIX configuration file name")
	ordcfg = flag.String("ordcfg", "demo_fix_orders.cfg", "Order flow FIX configuration file name")
	//flag.StringVar(&logfile, "logfile", "logs/debug.log", "path to the log file")
	//flag.StringVar(&configfile, "configfile", "config/server.cfg", "path to the config file")
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
	MD     *fix.FIX
	Orders *fix.FIX

	peerMutex sync.Mutex
	peers     map[string]*service.Peer

	factory service.ClientFactory
}

func (g *Gateway) AddPeer(fixSessionID string) {
	peer := service.NewPeer(g.factory)
	g.peers[fixSessionID] = peer
}

func (g *Gateway) FindPeer(fixSessionID string) (p *service.Peer, ok bool) {
	g.peerMutex.Lock()
	defer g.peerMutex.Unlock()
	p, ok = g.peers[fixSessionID]
	return
}

func (g *Gateway) RemovePeer(fixSessionID string) bool {
	g.peerMutex.Lock()
	defer g.peerMutex.Unlock()
	if p, ok := g.peers[fixSessionID]; ok {
		p.Close()
		delete(g.peers, fixSessionID)
		return true
	}
	return false
}

func (g *Gateway) Start() error {
	md := g.MD.Up()
	if md != nil {
		return md
	}
	ord := g.Orders.Up()
	if ord != nil {
		return ord
	}
	return nil
}

func (g *Gateway) Stop() {
	g.Orders.Down()
	g.MD.Down()
}

func newGateway(mdSettings, orderSettings *quickfix.Settings, factory service.ClientFactory) (*Gateway, bool) {
	g := &Gateway{
		logger:  log.Logger,
		peers:   make(map[string]*service.Peer),
		factory: factory,
	}
	md, err := fix.NewFIX(mdSettings, g)
	if err != nil {
		log.Logger.Fatal("create market data FIX", zap.Error(err))
		return nil, false
	}
	g.MD = md
	ord, err := fix.NewFIX(orderSettings, g)
	if err != nil {
		log.Logger.Fatal("create order flow FIX", zap.Error(err))
		return nil, false
	}
	g.Orders = ord
	return g, true
}

type defaultClientFactory struct {
	*websocket.Parameters
}

func (d *defaultClientFactory) NewWs() *websocket.Client {
	if d.Parameters == nil {
		d.Parameters = websocket.NewDefaultParameters()
	}
	return websocket.NewWithParams(d.Parameters)
}

func (d *defaultClientFactory) NewRest() *rest.Client {
	return rest.NewClient()
}

func main() {
	flag.Parse()

	mdf, err := os.Open(path.Join(FIXConfigDirectory, *mdcfg))
	if err != nil {
		log.Logger.Fatal("FIX market data config", zap.Error(err))
	}
	mds, err := quickfix.ParseSettings(mdf)
	if err != nil {
		log.Logger.Fatal("parse FIX market data settings", zap.Error(err))
	}
	ordf, err := os.Open(path.Join(FIXConfigDirectory, *ordcfg))
	if err != nil {
		log.Logger.Fatal("FIX order flow config", zap.Error(err))
	}
	ords, err := quickfix.ParseSettings(ordf)
	if err != nil {
		log.Logger.Fatal("parse FIX order flow settings", zap.Error(err))
	}
	factory := &defaultClientFactory{}
	g, ok := newGateway(mds, ords, factory)
	if !ok {
		return
	}
	err = g.Start()
	if err != nil {
		log.Logger.Fatal("start FIX", zap.Error(err))
	}

	g.logger.Info("starting stat server")

	// TODO remove profiling below for deployments
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
