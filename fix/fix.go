package fix

import (
	//"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bitfinexcom/bfxfixgw/log"

	bfxV1 "github.com/bitfinexcom/bitfinex-api-go/v1"
	bfx "github.com/bitfinexcom/bitfinex-api-go/v2"
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
	_ "github.com/shopspring/decimal"
)

type FIX struct {
	mu sync.Mutex // Mutex to protect the marketDataSubscriptions.
	*quickfix.MessageRouter

	bfx   *bfx.Client
	bfxV1 *bfxV1.Client

	bfxWSSDone <-chan struct{}
	bfxUserID  string

	marketDataSubscriptions map[string]*bfx.PublicSubscriptionRequest

	acc *quickfix.Acceptor

	logger *zap.Logger
}

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	b := bfx.NewClient().Credentials(sID.SenderCompID, sID.TargetCompID)
	f.bfx = b

	b1 := bfxV1.NewClient().Auth(sID.SenderCompID, sID.TargetCompID)
	f.bfxV1 = b1

	err := f.bfx.Websocket.Connect()
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
	}
	f.bfx.Websocket.SetReadTimeout(3 * time.Second)

	f.bfxWSSDone = f.bfx.Websocket.Done()

	go func() {
		var handler func(o interface{}, sID quickfix.SessionID)

		switch sID.BeginString {
		case quickfix.BeginStringFIX44:
			handler = f.FIX44Handler
		case quickfix.BeginStringFIX42:
			handler = f.FIX42Handler
		default:
			return // Unsupported
		}

		handler(nil, sID)
	}()

	return
}

// This waits for the Authentication event and then sets the User ID.
func (f *FIX) waitForUserID() error {
	wg := sync.WaitGroup{}
	wg.Add(1) // Ghetto once handler until once handlers get to the lib
	f.bfx.Websocket.AttachEventHandler(func(ev interface{}) {
		switch e := ev.(type) {
		case bfx.AuthEvent:
			if e.Status == "OK" {
				f.bfxUserID = strconv.FormatInt(e.UserID, 10)
				wg.Done()
			}
		}
	})
	defer f.bfx.Websocket.RemoveEventHandler()

	err := wait(&wg, 4*time.Second)
	return err
}

func (f *FIX) OnLogon(sID quickfix.SessionID) {
	log.Logger.Info("FIX.OnLogon", zap.Error(nil))
	return
}

func (f *FIX) OnLogout(sID quickfix.SessionID)                           { return }
func (f *FIX) ToAdmin(msg *quickfix.Message, sID quickfix.SessionID)     { return }
func (f *FIX) ToApp(msg *quickfix.Message, sID quickfix.SessionID) error { return nil }
func (f *FIX) FromAdmin(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	return nil
}

func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	return f.Route(msg, sID)
}

func NewFIX(s *quickfix.Settings) (*FIX, error) {
	f := &FIX{
		MessageRouter:           quickfix.NewMessageRouter(),
		marketDataSubscriptions: map[string]*bfx.PublicSubscriptionRequest{},
		bfxWSSDone:              make(<-chan struct{}),
		logger:                  log.Logger,
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

func (f *FIX) Up() error {
	if err := f.acc.Start(); err != nil {
		return err
	}

	return nil
}

func (f *FIX) Down() {
	f.acc.Stop()
}
