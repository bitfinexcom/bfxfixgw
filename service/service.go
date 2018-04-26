package service

import (
	lg "github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"github.com/bitfinexcom/bfxfixgw/service/websocket"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	wsv2 "github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	bmr "github.com/quickfixgo/fix42/businessmessagereject"
	mdrr "github.com/quickfixgo/fix42/marketdatarequestreject"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"log"
	"sync"
)

const TagMDRequestType quickfix.Tag = 20004

// LogicalService connects a logical FIX endpoint with a logical websocket connection
type Service struct {
	factory     peer.ClientFactory
	peers       map[string]*peer.Peer
	serviceType fix.FIXServiceType
	*fix.FIX
	*websocket.Websocket
	lock    sync.Mutex
	log     *zap.Logger
	inbound chan *peer.Message
}

func New(factory peer.ClientFactory, settings *quickfix.Settings, srvType fix.FIXServiceType, symbology symbol.Symbology) (*Service, error) {
	service := &Service{factory: factory, log: lg.Logger, peers: make(map[string]*peer.Peer), inbound: make(chan *peer.Message), serviceType: srvType}
	var err error
	service.FIX, err = fix.New(settings, service, srvType, symbology)
	if err != nil {
		lg.Logger.Fatal("create FIX", zap.Error(err))
		return nil, err
	}
	service.Websocket = websocket.New(service, symbology)
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

func (s *Service) AddPeer(fixSessionID quickfix.SessionID) *peer.Peer {
	p := peer.New(s.factory, fixSessionID, s.inbound)
	s.lock.Lock()
	s.peers[fixSessionID.String()] = p
	s.lock.Unlock()
	return p
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

func (s *Service) isMarketDataService() bool {
	return s.serviceType == fix.MarketDataService
}

func (s *Service) isOrderRoutingService() bool {
	return s.serviceType == fix.OrderRoutingService
}

func businessRejectReason(err string) enum.BusinessRejectReason {
	switch err {
	case "symbol: invalid":
		return enum.BusinessRejectReason_UNKNOWN_SECURITY
	default:
		return enum.BusinessRejectReason_OTHER
	}
}

func (s *Service) listen() {
	for msg := range s.inbound {
		if msg == nil {
			break
		}
		switch obj := msg.Data.(type) {
		case *bitfinex.Notification:
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42NotificationHandler(obj, msg.FIXSessionID())
		case *bitfinex.OrderNew:
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42OrderNewHandler(obj, msg.FIXSessionID())
		case *bitfinex.OrderCancel:
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42OrderCancelHandler(obj, msg.FIXSessionID())
		case *bitfinex.OrderUpdate:
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42OrderUpdateHandler(obj, msg.FIXSessionID())
		case *wsv2.InfoEvent:
			// no-op
		case *wsv2.AuthEvent:
			s.Websocket.FIX42HandleAuth(obj, msg.FIXSessionID())
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
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42OrderSnapshotHandler(obj, msg.FIXSessionID())
		case *wsv2.SubscribeEvent:
			// no-op: don't need to ack subscription to client
		case *bitfinex.BookUpdateSnapshot:
			if !s.isMarketDataService() {
				continue
			}
			s.Websocket.FIX42BookSnapshot(obj, msg.FIXSessionID())
		case *bitfinex.BookUpdate:
			if !s.isMarketDataService() {
				continue
			}
			s.Websocket.FIX42BookUpdate(obj, msg.FIXSessionID())
		case *bitfinex.TradeExecution:
			// 'te' delivered in-order
			/*
				if !s.isOrderRoutingService() {
					continue
				}
				s.Websocket.FIX42TradeExecutionHandler(obj, msg.FIXSessionID())
			*/
		case *bitfinex.TradeExecutionUpdate:
			// ignore trade execution update ('tu') in favor of trade executions since they come in-order
			if !s.isOrderRoutingService() {
				continue
			}
			s.Websocket.FIX42TradeExecutionUpdateHandler(obj, msg.FIXSessionID())
		case *bitfinex.Trade: // public trade
			if !s.isMarketDataService() {
				continue
			}
			s.Websocket.FIX42TradeHandler(obj, msg.FIXSessionID())
		case *bitfinex.TradeSnapshot:
			// no-op: do not provide trade snapshots
		case *wsv2.ErrorEvent:
			// subscription error
			if obj.SubID != "" {
				peer, ok := s.FindPeer(msg.FIXSessionID().String())
				if ok {
					_, err := peer.Ws.LookupSubscription(obj.SubID)
					if err == nil { // if sub exists, we know ref msg = V
						if fixReqID, ok := peer.ReverseLookupAPIReqIDs(obj.SubID); ok {
							fix := mdrr.New(field.NewMDReqID(fixReqID))
							fix.SetMDReqRejReason(enum.MDReqRejReason_UNKNOWN_SYMBOL)
							fix.SetText(obj.Message)
							fix.SetString(TagMDRequestType, obj.Channel)
							quickfix.SendToTarget(fix, msg.FIXSessionID())
							continue
						}
					}
				}
			}
			// generic error
			refMsgType := field.NewRefMsgType(s.FIX.LastMsgType()) // guess this is related to the last inbound FIX message
			reason := field.NewBusinessRejectReason(businessRejectReason(obj.Message))
			fix := bmr.New(refMsgType, reason)
			fix.SetText(obj.Message)
			quickfix.SendToTarget(fix, msg.FIXSessionID())
		case error:
			s.log.Error("processing error", zap.Any("msg", obj))
		default:
			s.log.Warn("unhandled message", zap.Any("msg", obj))
			log.Printf("%#v", obj)
		}
	}
}
