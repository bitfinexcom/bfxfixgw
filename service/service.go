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

// TagMDRequestType is the tag used for market data request type
const TagMDRequestType quickfix.Tag = 20004

// Service connects a logical FIX endpoint with a logical websocket connection
type Service struct {
	factory     peer.ClientFactory
	peers       map[string]*peer.Peer
	serviceType fix.ServiceType
	*fix.FIX
	*websocket.Websocket
	lock    sync.Mutex
	log     *zap.Logger
	inbound chan *peer.Message
}

// New creates a new service
func New(factory peer.ClientFactory, settings *quickfix.Settings, srvType fix.ServiceType, symbology symbol.Symbology) (*Service, error) {
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

// Start commences service operation
func (s *Service) Start() error {
	go s.listen()
	return s.FIX.Up()
}

// Stop ceases service operation
func (s *Service) Stop() {
	s.FIX.Down()
	s.lock.Lock()
	for _, p := range s.peers {
		p.Close()
	}
	close(s.inbound)
	s.lock.Unlock()
}

// AddPeer adds a FIX session to the current peer cache
func (s *Service) AddPeer(fixSessionID quickfix.SessionID) *peer.Peer {
	p := peer.New(s.factory, fixSessionID, s.inbound)
	s.lock.Lock()
	s.peers[fixSessionID.String()] = p
	s.lock.Unlock()
	return p
}

// FindPeer finds a FIX session in the current peer cache
func (s *Service) FindPeer(fixSessionID string) (*peer.Peer, bool) {
	s.lock.Lock()
	p, ok := s.peers[fixSessionID]
	s.lock.Unlock()
	return p, ok
}

// RemovePeer removes a FIX session from the current peer cache
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
			} else if err := s.Websocket.FIXNotificationHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix notification handler error", zap.Error(err))
			}
		case *bitfinex.OrderNew:
			if !s.isOrderRoutingService() {
				continue
			} else if err := s.Websocket.FIXOrderNewHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix order new handler error", zap.Error(err))
			}
		case *bitfinex.OrderCancel:
			if !s.isOrderRoutingService() {
				continue
			} else if err := s.Websocket.FIXOrderCancelHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix order cancel handler error", zap.Error(err))
			}
		case *bitfinex.OrderUpdate:
			if !s.isOrderRoutingService() {
				continue
			} else if err := s.Websocket.FIXOrderUpdateHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix order update handler error", zap.Error(err))
			}
		case *wsv2.InfoEvent:
			// no-op
		case *wsv2.AuthEvent:
			if err := s.Websocket.FIXHandleAuth(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix auth handler error", zap.Error(err))
			}
		case *bitfinex.FundingInfo:
			// no-op
		case *bitfinex.MarginInfoUpdate:
			// no-op
		case *bitfinex.MarginInfoBase:
			// no-op
		case *bitfinex.WalletSnapshot:
			if !s.isOrderRoutingService() {
				continue
			} else if err := s.Websocket.FIXWalletSnapshotHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix wallet snapshot handler error", zap.Error(err))
			}
		case *bitfinex.WalletUpdate:
			if !s.isOrderRoutingService() {
				continue
			} else if err := s.Websocket.FIXWalletUpdateHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix wallet update handler error", zap.Error(err))
			}
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
			} else if err := s.Websocket.FIXOrderSnapshotHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix order snapshot handler error", zap.Error(err))
			}
		case *wsv2.SubscribeEvent:
			// no-op: don't need to ack subscription to client
		case *bitfinex.BookUpdateSnapshot:
			if !s.isMarketDataService() {
				continue
			} else if err := s.Websocket.FIXBookSnapshot(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix book snapshot handler error", zap.Error(err))
			}
		case *bitfinex.BookUpdate:
			if !s.isMarketDataService() {
				continue
			} else if err := s.Websocket.FIXBookUpdate(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix book update handler error", zap.Error(err))
			}
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
			} else if err := s.Websocket.FIXTradeExecutionUpdateHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix trade execution update handler error", zap.Error(err))
			}
		case *bitfinex.Trade: // public trade
			if !s.isMarketDataService() {
				continue
			} else if err := s.Websocket.FIXTradeHandler(obj, msg.FIXSessionID()); err != nil {
				s.log.Error("fix trade handler error", zap.Error(err))
			}
		case *bitfinex.TradeSnapshot:
			// no-op: do not provide trade snapshots
		case *wsv2.ErrorEvent:
			// subscription error
			if obj.SubID != "" {
				peerFound, ok := s.FindPeer(msg.FIXSessionID().String())
				if ok {
					_, err := peerFound.Ws.LookupSubscription(obj.SubID)
					if err == nil { // if sub exists, we know ref msg = V
						if fixReqID, ok := peerFound.ReverseLookupAPIReqIDs(obj.SubID); ok {
							fixMsg := mdrr.New(field.NewMDReqID(fixReqID))
							fixMsg.SetMDReqRejReason(enum.MDReqRejReason_UNKNOWN_SYMBOL)
							fixMsg.SetText(obj.Message)
							fixMsg.SetString(TagMDRequestType, obj.Channel)
							if err = quickfix.SendToTarget(fixMsg, msg.FIXSessionID()); err != nil {
								s.log.Error("fix delivery error", zap.Error(err))
							}
							continue
						}
					}
				}
			}
			// generic error
			refMsgType := field.NewRefMsgType(s.FIX.LastMsgType()) // guess this is related to the last inbound FIX message
			reason := field.NewBusinessRejectReason(businessRejectReason(obj.Message))
			fixMsg := bmr.New(refMsgType, reason)
			fixMsg.SetText(obj.Message)
			if err := quickfix.SendToTarget(fixMsg, msg.FIXSessionID()); err != nil {
				s.log.Error("fix delivery error", zap.Error(err))
			}
		case error:
			s.log.Error("processing error", zap.Any("msg", obj))
		default:
			s.log.Warn("unhandled message", zap.Any("msg", obj))
			log.Printf("%#v", obj)
		}
	}
}
