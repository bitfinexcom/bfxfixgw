// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"sync"

	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"

	"github.com/bitfinexcom/bfxfixgw/fix"
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service"

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

	peerMutex sync.Mutex
	peers     map[string]*service.Peer

	factory service.ClientFactory
}

func (g *Gateway) AddPeer(id string) {
	peer := service.NewPeer(g.factory)
	g.peers[id] = peer
}

func (g *Gateway) FindPeer(id string) (p *service.Peer, ok bool) {
	g.peerMutex.Lock()
	defer g.peerMutex.Unlock()
	p, ok = g.peers[id]
	return
}

func (g *Gateway) RemovePeer(id string) bool {
	g.peerMutex.Lock()
	defer g.peerMutex.Unlock()
	if p, ok := g.peers[id]; ok {
		p.Close()
		delete(g.peers, id)
		return true
	}
	return false
}

func (g *Gateway) Start() error {
	return g.FIX.Up()
}

func (g *Gateway) Stop() {
	g.FIX.Down()
}

func newGateway(s *quickfix.Settings, factory service.ClientFactory) (*Gateway, bool) {
	g := &Gateway{
		logger:  log.Logger,
		peers:   make(map[string]*service.Peer),
		factory: factory,
	}
	fix, err := fix.NewFIX(s, g)
	if err != nil {
		log.Logger.Fatal("create FIX", zap.Error(err))
		return nil, false
	}
	g.FIX = fix
	return g, true
}

type defaultClientFactory struct {
	*websocket.Parameters
}

func (d *defaultClientFactory) NewClient() *websocket.Client {
	if d.Parameters == nil {
		d.Parameters = websocket.NewDefaultParameters()
	}
	return websocket.New()
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
	g, ok := newGateway(s, &defaultClientFactory{})
	if !ok {
		return
	}
	err = g.Start()
	if err != nil {
		log.Logger.Fatal("start FIX", zap.Error(err))
	}

	g.logger.Info("starting stat server")
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
