package fix

import (
	"sync"

	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"

	"go.uber.org/zap"

	fix42mdr "github.com/quickfixgo/fix42/marketdatarequest"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
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

type FIXServiceType byte

const (
	MarketDataService FIXServiceType = iota
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

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnCreate", zap.Any("SessionID", sID))
}

func (f *FIX) OnLogon(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnLogon", zap.Error(nil))
}

func (f *FIX) OnLogout(sID quickfix.SessionID) {
	log.Logger.Info("logging off websocket peer", zap.String("SessionID", sID.String()))
	f.RemovePeer(sID.String())
}
func (f *FIX) ToAdmin(msg *quickfix.Message, sID quickfix.SessionID) {
	f.logger.Info("FIX.ToAdmin", zap.Any("msg", msg))
}
func (f *FIX) ToApp(msg *quickfix.Message, sID quickfix.SessionID) error { return nil }
func (f *FIX) FromAdmin(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromAdmin", zap.Any("msg", msg))

	if msg.IsMsgTypeOf(msgTypeLogon) {
		peer := f.Peers.AddPeer(sID)
		go func() {
			select {
			case dc := <-peer.ListenDisconnect():
				if dc {
					logout("downstream disconnect", sID)
				}
			}
		}()
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
			p.Logon(apiKey, apiSecret, bfxUserID, cod)
		} else {
			f.logger.Warn("could not find peer", zap.String("SessionID", sID.String()))
		}
	}
	return nil
}

func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromApp", zap.Any("msg", msg))
	f.msgTypeLock.Lock()
	f.lastMsgType, _ = msg.Header.GetString(quickfix.Tag(35))
	f.msgTypeLock.Unlock()
	return f.Route(msg, sID)
}

func (f *FIX) LastMsgType() string {
	f.msgTypeLock.RLock()
	defer f.msgTypeLock.RUnlock()
	return f.lastMsgType
}

// New creates a new FIX acceptor & associated services
func New(s *quickfix.Settings, peers peer.Peers, serviceType FIXServiceType, symbology symbol.Symbology) (*FIX, error) {
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
