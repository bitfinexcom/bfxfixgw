package service

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/websocket"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	wsv2 "github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"sync"
)

// LogicalService connects a logical FIX endpoint with a logical websocket connection
type Service struct {
	factory peer.ClientFactory
	peers   map[string]*peer.Peer
	*fix.FIX
	*websocket.Websocket
	lock    sync.Mutex
	log     *zap.Logger
	inbound chan *peer.Message
}

func New(factory peer.ClientFactory, settings *quickfix.Settings, srvType fix.FIXServiceType) (*Service, error) {
	service := &Service{factory: factory, log: log.Logger, peers: make(map[string]*peer.Peer), inbound: make(chan *peer.Message)}
	var err error
	service.FIX, err = fix.New(settings, service, srvType)
	if err != nil {
		log.Logger.Fatal("create FIX", zap.Error(err))
		return nil, err
	}
	service.Websocket = websocket.New(service)
	return service, nil
}

func (s *Service) Start() error {
	go s.listen()
	return s.FIX.Up()
}

func (s *Service) Stop() {
	s.FIX.Down()
	s.lock.Lock()
	for _, p := range s.peers {
		p.Close()
	}
	close(s.inbound)
	s.lock.Unlock()
}

func (s *Service) AddPeer(fixSessionID quickfix.SessionID) {
	s.lock.Lock()
	s.peers[fixSessionID.String()] = peer.New(s.factory, fixSessionID, s.inbound)
	s.lock.Unlock()
}

func (s *Service) FindPeer(fixSessionID string) (*peer.Peer, bool) {
	s.lock.Lock()
	p, ok := s.peers[fixSessionID]
	s.lock.Unlock()
	return p, ok
}

func (s *Service) RemovePeer(fixSessionID string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	if p, ok := s.peers[fixSessionID]; ok {
		p.Close()
		delete(s.peers, fixSessionID)
		return true
	}
	return false
}

func (s *Service) listen() {
	for msg := range s.inbound {
		if msg == nil {
			break
		}
		switch obj := msg.Data.(type) {
		case *bitfinex.Notification:
			s.Websocket.FIX42NotificationHandler(obj, msg.FIXSessionID())
		case *wsv2.InfoEvent:
			// no-op
		case *wsv2.AuthEvent:
			// TODO log off FIX if this errors
		default:
			s.log.Warn("unhandled message", zap.Any("msg", msg.Data))
		}
	}
}
