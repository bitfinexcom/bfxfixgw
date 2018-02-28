package service

import (
	"github.com/bitfinexcom/bfxfixgw/log"

	"go.uber.org/zap"

	fix42mdr "github.com/quickfixgo/fix42/marketdatarequest"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42ocr "github.com/quickfixgo/fix42/ordercancelrequest"
	fix42osr "github.com/quickfixgo/fix42/orderstatusrequest"
	/*
		fix44mdr "github.com/quickfixgo/fix44/marketdatarequest"
		fix44nos "github.com/quickfixgo/fix44/newordersingle"
		fix44ocr "github.com/quickfixgo/fix44/ordercancelrequest"
		fix44osr "github.com/quickfixgo/fix44/orderstatusrequest"
	*/
	"github.com/quickfixgo/quickfix"
)

// send messages to FIX clients (global w/ session ID)
// send messages to websocket (peer map)

// FIX types, defined in BitfinexFIX42.xml
var msgTypeLogon = string([]byte("A"))
var tagBfxAPIKey = quickfix.Tag(20000)
var tagBfxAPISecret = quickfix.Tag(20001)
var tagBfxUserID = quickfix.Tag(20002)

type FIXServiceType byte

const (
	MarketDataService FIXServiceType = iota
	OrderRoutingService
)

// FIX establishes an acceptor and manages peer websocket clients
type FIX struct {
	*quickfix.MessageRouter

	Peers

	acc    *quickfix.Acceptor
	logger *zap.Logger
}

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnCreate", zap.Any("SessionID", sID))
	f.Peers.AddPeer(sID.String())
}

func (f *FIX) OnLogon(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnLogon", zap.Error(nil))
}

func (f *FIX) OnLogout(sID quickfix.SessionID) { return }
func (f *FIX) ToAdmin(msg *quickfix.Message, sID quickfix.SessionID) {
	f.logger.Info("FIX.ToAdmin", zap.Any("msg", msg))
}
func (f *FIX) ToApp(msg *quickfix.Message, sID quickfix.SessionID) error { return nil }
func (f *FIX) FromAdmin(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromAdmin", zap.Any("msg", msg))

	if msg.IsMsgTypeOf(msgTypeLogon) {
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
			p.Logon(apiKey, apiSecret, bfxUserID)
		} else {
			f.logger.Warn("could not find peer", zap.String("SessionID", sID.String()))
		}
	}
	return nil
}

func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	f.logger.Info("FIX.FromApp", zap.Any("msg", msg))
	return f.Route(msg, sID)
}

// NewFIX creates a new FIX acceptor & associated services
func NewFIX(s *quickfix.Settings, peers Peers, serviceType FIXServiceType) (*FIX, error) {
	f := &FIX{
		MessageRouter: quickfix.NewMessageRouter(),
		logger:        log.Logger,
		Peers:         peers,
	}

	/*
		// TODO FIX44
		f.AddRoute(fix44mdr.Route(f.OnFIX44MarketDataRequest))
		f.AddRoute(fix44nos.Route(f.OnFIX44NewOrderSingle))
		f.AddRoute(fix44ocr.Route(f.OnFIX44OrderCancelRequest))
		f.AddRoute(fix44osr.Route(f.OnFIX44OrderStatusRequest))
	*/
	lf := quickfix.NewScreenLogFactory()
	var factory quickfix.MessageStoreFactory
	if serviceType == OrderRoutingService {
		f.AddRoute(fix42nos.Route(f.OnFIX42NewOrderSingle))
		f.AddRoute(fix42ocr.Route(f.OnFIX42OrderCancelRequest))
		f.AddRoute(fix42osr.Route(f.OnFIX42OrderStatusRequest))
		factory = quickfix.NewFileStoreFactory(s)
	} else {
		f.AddRoute(fix42mdr.Route(f.OnFIX42MarketDataRequest))
		factory = NewNoStoreFactory()
	}

	a, err := quickfix.NewAcceptor(f, factory, s, lf)
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
