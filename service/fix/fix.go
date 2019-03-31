package fix

import (
	"sync"

	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"

	"go.uber.org/zap"

	fix42mdr "github.com/quickfixgo/fix42/marketdatarequest"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42ocrr "github.com/quickfixgo/fix42/ordercancelreplacerequest"
	fix42ocr "github.com/quickfixgo/fix42/ordercancelrequest"
	fix42osr "github.com/quickfixgo/fix42/orderstatusrequest"
	"github.com/quickfixgo/quickfix"
)

// FIX types, defined in BitfinexFIX42.xml
var msgTypeLogon = string([]byte("A"))
var tagBfxAPIKey = quickfix.Tag(20000)
var tagBfxAPISecret = quickfix.Tag(20001)
var tagBfxUserID = quickfix.Tag(20002)
var tagCancelOnDisconnect = quickfix.Tag(8013)

// ServiceType is the package service type
type ServiceType byte

const (
	// MarketDataService defines a MD service
	MarketDataService ServiceType = iota
	// OrderRoutingService defines an Order Routing service
	OrderRoutingService
)

// FIX establishes an acceptor and manages peer websocket clients
type FIX struct {
	*quickfix.MessageRouter

	peer.Peers
	symbol.Symbology

	acc    *quickfix.Acceptor
	logger *zap.Logger

	lastMsgType string
	msgTypeLock sync.RWMutex
}

// OnCreate handles FIX session creation
func (f *FIX) OnCreate(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnCreate", zap.Any("SessionID", sID))
}

// OnLogon handles FIX session logon
func (f *FIX) OnLogon(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnLogon", zap.Error(nil))
}

// OnLogout handles FIX session logout
func (f *FIX) OnLogout(sID quickfix.SessionID) {
	log.Logger.Info("logging off websocket peer", zap.String("SessionID", sID.String()))
	f.RemovePeer(sID.String())
}

// ToAdmin handles FIX admin message delivery
func (f *FIX) ToAdmin(msg *quickfix.Message, sID quickfix.SessionID) {
	f.logger.Info("FIX.ToAdmin", zap.Any("msg", msg))
}

// ToApp handles FIX app message delivery
func (f *FIX) ToApp(msg *quickfix.Message, sID quickfix.SessionID) error { return nil }

// FromAdmin handles FIX admin message processing
func (f *FIX) FromAdmin(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromAdmin", zap.Any("msg", msg))

	if msg.IsMsgTypeOf(msgTypeLogon) {
		peerAdded := f.Peers.AddPeer(sID)
		go func(session string) {
			dc := <-peerAdded.ListenDisconnect()
			if _, ok := f.FindPeer(session); dc && ok {
				if errReportDisconnect := logout("downstream disconnect", sID); errReportDisconnect != nil {
					//If disconnect cannot be reported, we are in unrecoverable state
					//Best to panic and let the gateway come back online
					panic(errReportDisconnect)
				}
			}
		}(sID.String())
		apiKey, err := msg.Body.GetString(tagBfxAPIKey)
		if err != nil || apiKey == "" {
			f.logger.Warn("received Logon without BfxApiKey (20000)", zap.Error(err))
			return err
		}
		apiSecret, err := msg.Body.GetString(tagBfxAPISecret)
		if err != nil || apiSecret == "" {
			f.logger.Warn("received Logon without BfxApiSecret (20001)", zap.Error(err))
			return err
		}
		bfxUserID, err := msg.Body.GetString(tagBfxUserID)
		if err != nil || bfxUserID == "" {
			f.logger.Warn("received Logon without BfxUserID (20002)", zap.Error(err))
			return err
		}
		if p, ok := f.FindPeer(sID.String()); ok {
			cod, _ := msg.Body.GetBool(tagCancelOnDisconnect)
			err := p.Logon(apiKey, apiSecret, bfxUserID, cod)
			if err != nil {
				if err = logout(err.Error(), sID); err != nil {
					return reject(err)
				}
			}
		} else {
			f.logger.Warn("could not find peer", zap.String("SessionID", sID.String()))
		}
	}
	return nil
}

// FromApp handles FIX application message processing
func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromApp", zap.Any("msg", msg))
	f.msgTypeLock.Lock()
	f.lastMsgType, _ = msg.Header.GetString(quickfix.Tag(35))
	f.msgTypeLock.Unlock()
	return f.Route(msg, sID)
}

// LastMsgType retrieves the last message type
func (f *FIX) LastMsgType() string {
	f.msgTypeLock.RLock()
	defer f.msgTypeLock.RUnlock()
	return f.lastMsgType
}

// New creates a new FIX acceptor & associated services
func New(s *quickfix.Settings, peers peer.Peers, serviceType ServiceType, symbology symbol.Symbology) (*FIX, error) {
	f := &FIX{
		MessageRouter: quickfix.NewMessageRouter(),
		logger:        log.Logger,
		Peers:         peers,
		Symbology:     symbology,
	}

	var storeFactory quickfix.MessageStoreFactory
	logFactory, err := quickfix.NewFileLogFactory(s)
	if err != nil {
		return nil, err
	}
	if serviceType == OrderRoutingService {
		f.AddRoute(fix42nos.Route(f.OnFIX42NewOrderSingle))
		f.AddRoute(fix42ocrr.Route(f.OnFIX42OrderCancelReplaceRequest))
		f.AddRoute(fix42ocr.Route(f.OnFIX42OrderCancelRequest))
		f.AddRoute(fix42osr.Route(f.OnFIX42OrderStatusRequest))
		storeFactory = quickfix.NewFileStoreFactory(s)
	} else {
		f.AddRoute(fix42mdr.Route(f.OnFIX42MarketDataRequest))
		storeFactory = NewNoStoreFactory()
	}

	a, err := quickfix.NewAcceptor(f, storeFactory, s, logFactory)
	if err != nil {
		return nil, err
	}

	f.acc = a

	return f, nil
}

// Up starts the FIX acceptor service
func (f *FIX) Up() error {
	return f.acc.Start()
}

// Down stops the FIX acceptor service
func (f *FIX) Down() {
	f.acc.Stop()
}
