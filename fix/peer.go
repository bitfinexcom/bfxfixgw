package fix

import (
	"context"
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"time"
)

// Peer represents a FIX-websocket peer user
type Peer struct {
	SessionID     quickfix.SessionID
	subscriptions map[string]*subscription
	bfx           *websocket.Client
	logger        *zap.Logger
	bfxUserID     string
}

type subscription struct {
	Request        *websocket.SubscriptionRequest
	SubscriptionID string
}

// creates a peer, but does not establish a websocket connection yet
func newPeer(sID quickfix.SessionID) *Peer {
	return &Peer{
		SessionID:     sID,
		subscriptions: make(map[string]*subscription),
		bfx:           websocket.NewClient(),
		logger:        log.Logger,
	}
}

// Logon establishes a websocket connection and attempts to authenticate with the given apiKey and apiSecret
func (p *Peer) Logon(apiKey, apiSecret, bfxUserID string) error {
	p.bfx.Credentials(apiKey, apiSecret)
	p.bfxUserID = bfxUserID
	err := p.bfx.Connect()
	if err != nil {
		return err
	}
	go p.listen()
	return nil
}

func (p *Peer) subscribe() error {
	for _, v := range p.subscriptions {
		ctx, cxl := context.WithTimeout(context.Background(), time.Second*2)
		defer cxl()
		id, err := p.bfx.Subscribe(ctx, v.Request)
		if err != nil {
			return err
		}
		v.SubscriptionID = id
	}
	return nil
}

func (p *Peer) unsubscribe() error {
	for _, v := range p.subscriptions {
		ctx, cxl := context.WithTimeout(context.Background(), time.Second*2)
		defer cxl()
		err := p.bfx.Unsubscribe(ctx, v.SubscriptionID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Peer) resubscribe() error {
	if err := p.unsubscribe(); err != nil {
		return err
	}
	return p.subscribe()
}

func (p *Peer) listen() {
	for msg := range p.bfx.Listen() {
		if msg == nil {
			p.logger.Info("upstream disconnect")
			// TODO log out peer
			return
		}
		switch msg.(type) {
		case *websocket.AuthEvent:
			err := p.subscribe()
			if err != nil {
				p.logger.Error("could not subscribe", zap.Any("msg", msg))
			}
		case *bitfinex.BookUpdate:
			// TODO
		default:
			p.logger.Error("unhandled event: %#v", zap.Any("msg", msg))
		}
	}
}

// BfxUserID is an immutable accessor to the bitfinex user ID
func (p *Peer) BfxUserID() string {
	return p.bfxUserID
}