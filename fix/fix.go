package fix

import (
	"sync"
	"time"

	"github.com/bitfinexcom/bfxfixgw/log"

	"go.uber.org/zap"

	fix42mdr "github.com/quickfixgo/fix42/marketdatarequest"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42ocr "github.com/quickfixgo/fix42/ordercancelrequest"
	fix42osr "github.com/quickfixgo/fix42/orderstatusrequest"

	fix44mdr "github.com/quickfixgo/fix44/marketdatarequest"
	fix44nos "github.com/quickfixgo/fix44/newordersingle"
	fix44ocr "github.com/quickfixgo/fix44/ordercancelrequest"
	fix44osr "github.com/quickfixgo/fix44/orderstatusrequest"

	"github.com/quickfixgo/quickfix"
)

// FIX types, defined in BitfinexFIX42.xml
var msgTypeLogon = string([]byte("A"))
var tagBfxAPIKey = quickfix.Tag(20000)
var tagBfxAPISecret = quickfix.Tag(20001)
var tagBfxUserID = quickfix.Tag(20002)

// FIX establishes an acceptor and manages peer websocket clients
type FIX struct {
	*quickfix.MessageRouter

	// signaling to control peer threading model
	peerMutex sync.Mutex
	peers     map[quickfix.SessionID]*Peer

	acc    *quickfix.Acceptor
	logger *zap.Logger
}

func (f *FIX) makePeer(sID quickfix.SessionID) error {
	peer := newPeer(sID)
	f.peers[sID] = peer

	peer.bfx.SetReadTimeout(8 * time.Second)
	err := peer.bfx.Connect()
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return err
	}

	return nil
}

func (f *FIX) peer(sID quickfix.SessionID) (p *Peer, ok bool) {
	f.peerMutex.Lock()
	defer f.peerMutex.Unlock()
	p, ok = f.peers[sID]
	return
}

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	f.peerMutex.Lock()
	f.peers[sID] = newPeer(sID)
	f.peerMutex.Unlock()
	return
}

func (f *FIX) OnLogon(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnLogon", zap.Error(nil))
	return
}

func (f *FIX) OnLogout(sID quickfix.SessionID)                           { return }
func (f *FIX) ToAdmin(msg *quickfix.Message, sID quickfix.SessionID)     { return }
func (f *FIX) ToApp(msg *quickfix.Message, sID quickfix.SessionID) error { return nil }
func (f *FIX) FromAdmin(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	if msg.IsMsgTypeOf(msgTypeLogon) {
		f.peerMutex.Lock()
		apiKey, err := msg.Body.GetString(tagBfxAPIKey)
		if err != nil {
			log.Logger.Warn("received Logon without BfxApiKey (20000)", zap.Error(err))
			return err
		}
		apiSecret, err := msg.Body.GetString(tagBfxAPISecret)
		if err != nil {
			log.Logger.Warn("received Logon without BfxApiSecret (20001)", zap.Error(err))
			return err
		}
		bfxUserID, err := msg.Body.GetString(tagBfxUserID)
		if err != nil {
			log.Logger.Warn("received Logon without BfxUserID (20002)", zap.Error(err))
			return err
		}
		f.peers[sID].Logon(apiKey, apiSecret, bfxUserID)
		f.peerMutex.Unlock()
	}
	return nil
}

func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	return f.Route(msg, sID)
}

// NewFIX creates a new FIX acceptor & associated services
func NewFIX(s *quickfix.Settings) (*FIX, error) {
	f := &FIX{
		MessageRouter: quickfix.NewMessageRouter(),
		logger:        log.Logger,
		peers:         make(map[quickfix.SessionID]*Peer),
	}

	f.AddRoute(fix42mdr.Route(f.OnFIX42MarketDataRequest))
	f.AddRoute(fix42nos.Route(f.OnFIX42NewOrderSingle))
	f.AddRoute(fix42ocr.Route(f.OnFIX42OrderCancelRequest))
	f.AddRoute(fix42osr.Route(f.OnFIX42OrderStatusRequest))

	f.AddRoute(fix44mdr.Route(f.OnFIX44MarketDataRequest))
	f.AddRoute(fix44nos.Route(f.OnFIX44NewOrderSingle))
	f.AddRoute(fix44ocr.Route(f.OnFIX44OrderCancelRequest))
	f.AddRoute(fix44osr.Route(f.OnFIX44OrderStatusRequest))

	lf := quickfix.NewScreenLogFactory()
	a, err := quickfix.NewAcceptor(f, quickfix.NewMemoryStoreFactory(), s, lf)
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
