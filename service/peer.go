package service

import (
	"context"
	"log"
	"time"

	bfxlog "github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"go.uber.org/zap"
)

type ClientFactory interface {
	Create() *websocket.Client
}

// Peers is an interface to create, remove, and lookup peers.
type Peers interface {
	FindPeer(id string) (*Peer, bool)
	RemovePeer(id string) bool
	AddPeer(id string)
}

// Peer represents a FIX-websocket peer user
type Peer struct {
	subscriptions map[string]*subscription
	Bfx           *websocket.Client
	logger        *zap.Logger

	bfxUserID string
}

type subscription struct {
	Request        *websocket.SubscriptionRequest
	SubscriptionID string
}

// NewPeer creates a peer, but does not establish a websocket connection yet
func NewPeer(factory ClientFactory) *Peer {
	return &Peer{
		subscriptions: make(map[string]*subscription),
		Bfx:           factory.Create(),
		logger:        bfxlog.Logger,
	}
}

// Logon establishes a websocket connection and attempts to authenticate with the given apiKey and apiSecret
func (p *Peer) Logon(apiKey, apiSecret, bfxUserID string) error {
	p.Bfx.Credentials(apiKey, apiSecret)
	p.bfxUserID = bfxUserID
	err := p.Bfx.Connect()
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
		id, err := p.Bfx.Subscribe(ctx, v.Request)
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
		err := p.Bfx.Unsubscribe(ctx, v.SubscriptionID)
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
	for msg := range p.Bfx.Listen() {
		log.Printf("peer got msg: %#v", msg)
		if msg == nil {
			p.logger.Info("upstream disconnect")
			// TODO log out peer
			return
		}
		switch msg.(type) {
		case *websocket.InfoEvent:
			// TODO logon? no logon--client has not yet set credentials
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

func (p *Peer) Close() {
	p.Bfx.Close()
}
