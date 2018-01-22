package fix

import (
	"context"
	"sync"
	"time"

	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
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
	*quickfix.MessageRouter

	bfxMu sync.Mutex
	bfx   *websocket.Client

	bfxUserID       string
	isConnected     bool
	maintenanceMode bool

	MDMu                    sync.Mutex // Mutex to protect the marketDataSubscriptions.
	marketDataSubscriptions map[string]*BfxSubscription

	acc *quickfix.Acceptor

	logger *zap.Logger
}

type BfxSubscription struct {
	Request        *websocket.SubscriptionRequest
	SubscriptionID string
}

// TODO move to OnLogon
func (f *FIX) OnCreate(sID quickfix.SessionID) {
	err := f.initBfx(sID)
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return
	}
	// TODO go listen per client

	// TODO attach handlers?

	return
}

func (f *FIX) initBfx(sID quickfix.SessionID) error {
	f.bfx = websocket.NewClient().Credentials(sID.SenderCompID, sID.TargetCompID)

	go f.listen()
	f.bfx.SetReadTimeout(8 * time.Second)
	err := f.bfx.Connect()
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return err
	}

	return nil
}

// TODO mux to FIX clients
func (f *FIX) listen() {
	for msg := range f.bfx.Listen() {
		if msg == nil {
			f.logger.Info("upstream disconnect")
			return
		}
		switch msg.(type) {
		case *websocket.AuthEvent:
			err := f.subscribe()
			if err != nil {
				f.logger.Error("could not subscribe", zap.Any("msg", msg))
			}
		case *bitfinex.BookUpdate:
			// TODO
		default:
			f.logger.Error("unhandled event: %#v", zap.Any("msg", msg))
		}
	}
}

/*
func (f *FIX) attachHandlers(sID quickfix.SessionID) error {
	var handler func(o interface{}, sID quickfix.SessionID)

	switch sID.BeginString {
	case quickfix.BeginStringFIX44:
		handler = f.FIX44Handler
	case quickfix.BeginStringFIX42:
		handler = f.FIX42Handler
	default:
		return fmt.Errorf("unsupported FIX version: %s", sID.BeginString)
	}

	// TODO attach handlers

	return nil
}
*/

func (f *FIX) subscribe() error {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*1)

	for _, v := range f.marketDataSubscriptions {
		ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
		id, err := f.bfx.Subscribe(ctx, v.Request)
		if err != nil {
			return err
		}
		v.SubscriptionID = id
	}

	return nil
}

func (f *FIX) unsubscribe() error {
	for _, v := range f.marketDataSubscriptions {
		ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
		err := f.bfx.Unsubscribe(ctx, v.SubscriptionID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *FIX) resubscribe() error {
	if err := f.unsubscribe(); err != nil {
		return err
	}
	return f.subscribe()
}

func (f *FIX) inMaintenanceMode() bool {
	return f.maintenanceMode
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
		marketDataSubscriptions: make(map[string]*BfxSubscription),
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
