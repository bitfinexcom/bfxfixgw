package service

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"sync"
)

type FIXService interface {
	SubmitOrder(order *bitfinex.OrderNewRequest)
	SubmitCancel(cancel *bitfinex.OrderCancelRequest)
	OrderUpdates() *bitfinex.OrderUpdate
}

// LogicalService connects a logical FIX endpoint with a logical websocket connection
type Service struct {
	factory ClientFactory
	*FIX
	peers map[string]*Peer
	lock  sync.Mutex
	log   *zap.Logger
}

func New(factory ClientFactory, settings *quickfix.Settings, srvType FIXServiceType) (*Service, error) {
	service := &Service{factory: factory, log: log.Logger}
	var err error
	service.FIX, err = NewFIX(settings, service, srvType)
	if err != nil {
		log.Logger.Fatal("create FIX", zap.Error(err))
		return nil, err
	}
	return service, nil
}

func (s *Service) Start() error {
	return s.FIX.Up()
}

func (s *Service) Stop() {
	s.FIX.Down()
}

func (s *Service) AddPeer(fixSessionID string) {
	s.lock.Lock()
	s.peers[fixSessionID] = newPeer(s.factory, fixSessionID)
	s.lock.Unlock()
}

func (s *Service) FindPeer(fixSessionID string) (*Peer, bool) {
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
