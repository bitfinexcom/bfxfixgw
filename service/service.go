package service

import (
	lg "github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/websocket"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	wsv2 "github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"log"
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
	service := &Service{factory: factory, log: lg.Logger, peers: make(map[string]*peer.Peer), inbound: make(chan *peer.Message)}
	var err error
	service.FIX, err = fix.New(settings, service, srvType)
	if err != nil {
		lg.Logger.Fatal("create FIX", zap.Error(err))
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

func (s *Service) processOrderTerminal(o *bitfinex.OrderCancel, sid quickfix.SessionID) {
	/*
		peer, ok := s.FindPeer(sid.String())
		if !ok {
			s.log.Warn("could not find peer for SessionID", zap.String("SessionID", sid.String()))
			return
		}
		// TODO is this a cancel ack?
		orderID := strconv.FormatInt(o.ID, 10)
		clOrdID := strconv.FormatInt(o.CID, 10)
		peer.AddOrder(orderID, clOrdID, o.Price, o.Amount)
		// TODO generate "filled" ER or is this triggered by a tu with rem qty = 0?
	*/
	// this is handled by the last 'tu' message
}

func (s *Service) listen() {
	for msg := range s.inbound {
		if msg == nil {
			break
		}
		switch obj := msg.Data.(type) {
		case *bitfinex.Notification:
			s.Websocket.FIX42NotificationHandler(obj, msg.FIXSessionID())
		case *bitfinex.OrderNew:
			s.Websocket.FIX42OrderNewHandler(obj, msg.FIXSessionID())
		case *bitfinex.OrderCancel:
			s.Websocket.FIX42OrderCancelHandler(obj, msg.FIXSessionID())
		case *wsv2.InfoEvent:
			// no-op
		case *wsv2.AuthEvent:
			// TODO log off FIX if this errors
			log.Printf("got auth event: %#v", obj)
		case *bitfinex.FundingInfo:
			// no-op
		case *bitfinex.MarginInfoUpdate:
			// no-op
		case *bitfinex.MarginInfoBase:
			// no-op
		case *bitfinex.WalletSnapshot:
			// no-op
		case *bitfinex.WalletUpdate:
			// no-op
		case *bitfinex.BalanceInfo:
			// no-op
		case *bitfinex.BalanceUpdate:
			// no-op
		case *bitfinex.PositionSnapshot:
			// no-op
		case *bitfinex.PositionUpdate:
			// no-op
		case *bitfinex.OrderSnapshot:
			log.Printf("got order snapshot: %#v", obj)
			// TODO
		case *wsv2.SubscribeEvent:
			// TODO handle these or no?
		case *bitfinex.BookUpdateSnapshot:
			s.Websocket.FIX42BookSnapshot(obj, msg.FIXSessionID())
		case *bitfinex.BookUpdate:
			s.Websocket.FIX42BookUpdate(obj, msg.FIXSessionID())
		case *bitfinex.TradeExecution:
			// ignore trade executions in favor of trade execution updates (more data)
		case *bitfinex.TradeExecutionUpdate:
			s.Websocket.FIX42TradeExecutionUpdateHandler(obj, msg.FIXSessionID())
		case error:
			s.log.Error("processing error", zap.Any("msg", obj))
		default:
			s.log.Warn("unhandled message", zap.Any("msg", obj))
			log.Printf("%#v", obj)
		}
	}
}
