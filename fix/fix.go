package fix

import (
	"fmt"
	"sync"

	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/knarz/bitfinex-api-go"
	"github.com/uber-go/zap"

	fix42mdr "github.com/quickfixgo/quickfix/fix42/marketdatarequest"
	fix42nos "github.com/quickfixgo/quickfix/fix42/newordersingle"
	fix42ocr "github.com/quickfixgo/quickfix/fix42/ordercancelrequest"
	fix42osr "github.com/quickfixgo/quickfix/fix42/orderstatusrequest"

	fix44mdr "github.com/quickfixgo/quickfix/fix44/marketdatarequest"
	fix44nos "github.com/quickfixgo/quickfix/fix44/newordersingle"
	fix44ocr "github.com/quickfixgo/quickfix/fix44/ordercancelrequest"
	fix44osr "github.com/quickfixgo/quickfix/fix44/orderstatusrequest"

	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/quickfix/enum"
	_ "github.com/quickfixgo/quickfix/field"
	_ "github.com/quickfixgo/quickfix/tag"
	_ "github.com/shopspring/decimal"
)

// MarketDataChan provides a way to send market data between go routines.
type MarketDataChan struct {
	mu sync.Mutex
	c  chan []float64
	// C is used to send market data to. Use the Close method instead of calling
	// close() on it.
	C    chan<- []float64
	err  error
	done chan struct{}
}

// Done returns a channel that is closed once the channel is closed.
func (m *MarketDataChan) Done() <-chan struct{} {
	return m.done
}

func (m *MarketDataChan) Close(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
	close(m.c)
	close(m.done)
}

// Receive returns the channel from which to read MarketData.
func (m *MarketDataChan) Receive() <-chan []float64 { return m.c }

func (m *MarketDataChan) Send(md []float64) { m.c <- md }

func (m *MarketDataChan) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func NewMarketDataChan() *MarketDataChan {
	mdc := &MarketDataChan{
		c:    make(chan []float64),
		done: make(chan struct{}),
	}
	mdc.C = mdc.c

	return mdc
}

type FIX struct {
	mu sync.Mutex // Mutex to protect the marketDataChans.
	*quickfix.MessageRouter

	// XXX: these could also go into a map[quickfix.SessionID]
	bfx   *bitfinex.ClientV2
	bfxV1 *bitfinex.Client

	// XXX: make TermDataChan similar to the MarketDataChan.
	termDataChans     map[quickfix.SessionID]chan bitfinex.TermData
	bfxWSSDone        map[quickfix.SessionID]chan struct{}
	bfxPrivateWSSDone map[quickfix.SessionID]chan struct{}
	bfxUserIDs        map[quickfix.SessionID]int64

	// map[MDReqID]chan
	marketDataChans map[string]*MarketDataChan

	acc *quickfix.Acceptor

	logger zap.Logger
}

func (f *FIX) OnCreate(sID quickfix.SessionID) {
	b := bitfinex.NewClientV2().Auth(sID.SenderCompID, sID.TargetCompID)
	f.bfx = b

	b1 := bitfinex.NewClient().Auth(sID.SenderCompID, sID.TargetCompID)
	f.bfxV1 = b1

	err := f.bfx.WebSocket.Connect()
	if err != nil {
		f.logger.Error("websocket connect", zap.Error(err))
	}

	f.bfxWSSDone[sID] = make(chan struct{})
	go func() {
		f.logger.Debug("websocket watch")
		err := f.bfx.WebSocket.Watch(f.bfxWSSDone[sID])
		if err != nil {
			// XXX: Handle this error
			f.logger.Error("websocket watch", zap.Error(err))
		}
	}()

	f.termDataChans[sID] = make(chan bitfinex.TermData)
	err = f.bfx.WebSocket.ConnectPrivateToken(sID.SenderCompID, f.termDataChans[sID])
	if err != nil {
		f.logger.Error("websocket connect private", zap.Error(err))
	}

	f.bfxUserIDs[sID] = int64(f.bfx.WebSocket.UserId)

	f.bfxPrivateWSSDone[sID] = make(chan struct{})
	go func() {
		f.logger.Debug("websocket watch private")
		err := f.bfx.WebSocket.WatchPrivate(f.bfxPrivateWSSDone[sID])
		if err != nil {
			f.logger.Error("websocket watch private", zap.Error(err))
		}
	}()

	go func() {
		var handler func(d bitfinex.TermData, sID quickfix.SessionID)

		switch sID.BeginString {
		case enum.BeginStringFIX44:
			handler = f.FIX44TermDataHandler
		case enum.BeginStringFIX42:
			handler = f.FIX42TermDataHandler
		default:
			return // Unsupported
		}

		for {
			select {
			case data := <-f.termDataChans[sID]:
				if data.HasError() {
					// Data has error - websocket channel will be closed.
					// XXX: Handle this properly.
					fmt.Println("wssReader:", data.Error)
					return
				} else {
					handler(data, sID)
				}
			}
		}
	}()

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
	return nil
}

func (f *FIX) FromApp(msg *quickfix.Message, sID quickfix.SessionID) quickfix.MessageRejectError {
	return f.Route(msg, sID)
}

func NewFIX(s *quickfix.Settings) (*FIX, error) {
	f := &FIX{
		MessageRouter:     quickfix.NewMessageRouter(),
		marketDataChans:   map[string]*MarketDataChan{},
		termDataChans:     map[quickfix.SessionID]chan bitfinex.TermData{},
		bfxWSSDone:        map[quickfix.SessionID]chan struct{}{},
		bfxPrivateWSSDone: map[quickfix.SessionID]chan struct{}{},
		bfxUserIDs:        map[quickfix.SessionID]int64{},
		logger:            log.Logger,
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
