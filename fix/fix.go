package fix

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
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
	bfx   *bitfinex.Client

	bfxUserID       string
	isConnected     bool
	maintenanceMode bool

	MDMu                    sync.Mutex // Mutex to protect the marketDataSubscriptions.
	marketDataSubscriptions map[string]BfxSubscription

	acc *quickfix.Acceptor

	logger *zap.Logger
}

type BfxSubscription struct {
	Request *bitfinex.PublicSubscriptionRequest
	Handler func(ev interface{})
}

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	err := f.initBfx(sID)
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return
	}
	go f.listener(sID)

	if err := f.attachHandlers(sID); err != nil {
		f.logger.Error("attaching handlers", zap.Error(err))
		return
	}

	if err := f.subscribe(); err != nil {
		f.logger.Error("websocket subscribe", zap.Error(err))
		return
	}

	return
}

func (f *FIX) initBfx(sID quickfix.SessionID) error {
	b := bitfinex.NewClient().Credentials(sID.SenderCompID, sID.TargetCompID)

	err := b.Websocket.Connect()
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return err
	}
	b.Websocket.SetReadTimeout(8 * time.Second)

	f.bfxMu.Lock()
	f.bfx = b
	f.bfxMu.Unlock()

	return nil
}

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

	f.bfx.Websocket.AttachPrivateHandler(func(ev interface{}) {
		handler(ev, sID)
	})

	f.bfx.Websocket.AttachEventHandler(func(ev interface{}) {
		switch e := ev.(type) {
		case bitfinex.InfoEvent:
			// TODO: handle maintenance mode/restarts
		}
	})

	return nil
}

func (f *FIX) subscribe() error {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*1)
	f.bfx.Websocket.Authenticate(ctx)
	if err := f.waitForUserID(); err != nil {
		return err
	}

	for _, v := range f.marketDataSubscriptions {
		ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
		err := f.bfx.Websocket.Subscribe(ctx, v.Request, v.Handler)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *FIX) unsubscribe() error {
	for _, v := range f.marketDataSubscriptions {
		ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
		err := f.bfx.Websocket.Unsubscribe(ctx, v.Request)
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

func (f *FIX) listener(sID quickfix.SessionID) {
	for {
		select {
		case <-f.bfx.Websocket.Done():
			f.logger.Error("ws quit", zap.Error(f.bfx.Websocket.Err()))
			f.isConnected = false
			defer f.reconnectWebsocket(sID)
			return
		}
	}
}

func (f *FIX) reconnectWebsocket(sID quickfix.SessionID) {
	err := f.initBfx(sID)
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
		return
	}

	if err := f.subscribe(); err != nil {
		f.logger.Error("websocket authenticate", zap.Error(err))
		return
	}

	if err := f.attachHandlers(sID); err != nil {
		f.logger.Error("attaching handlers", zap.Error(err))
		return
	}

	go f.listener(sID)
}

func (f *FIX) inMaintenanceMode() bool {
	return f.maintenanceMode
}

// This waits for the Authentication event and then sets the User ID.
func (f *FIX) waitForUserID() error {
	wg := sync.WaitGroup{}
	wg.Add(1) // Ghetto once handler until once handlers get to the lib
	f.bfx.Websocket.AttachEventHandler(func(ev interface{}) {
		switch e := ev.(type) {
		case bitfinex.AuthEvent:
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
		marketDataSubscriptions: map[string]BfxSubscription{},
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
