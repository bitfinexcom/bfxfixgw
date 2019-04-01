package peer

import (
	"log"

	bfxlog "github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

// ClientFactory is an interface to create new REST and WS clients
type ClientFactory interface {
	NewRest() *rest.Client
	NewWs() *websocket.Client
}

// Peers is an interface to create, remove, and lookup peers.
type Peers interface {
	FindPeer(id string) (*Peer, bool)
	RemovePeer(id string) bool
	AddPeer(id quickfix.SessionID) *Peer
}

// Message is a raw data container with an associated peer
type Message struct {
	Data interface{}
	*Peer
}

// Peer represents a FIX-websocket peer user
type Peer struct {
	Ws         *websocket.Client
	Rest       *rest.Client
	toParent   chan<- *Message
	exit       chan struct{}
	disconnect chan bool
	started    bool

	logger *zap.Logger

	bfxUserID string
	sessionID quickfix.SessionID
	*cache
}

// New creates a peer, but does not establish a websocket connection yet
func New(factory ClientFactory, fixSessionID quickfix.SessionID, toParent chan<- *Message) *Peer {
	log.Printf("created peer for %s", fixSessionID)
	return &Peer{
		Ws:         factory.NewWs(),
		Rest:       factory.NewRest(),
		logger:     bfxlog.Logger,
		sessionID:  fixSessionID,
		toParent:   toParent,
		exit:       make(chan struct{}),
		disconnect: make(chan bool),
		cache:      newCache(bfxlog.Logger),
		started:    false,
	}
}

// ListenDisconnect provides a channel for a caller to block on until this peer disconnects, sending a true value on this channel.
func (p *Peer) ListenDisconnect() <-chan bool {
	return p.disconnect
}

// Logon establishes a websocket connection and attempts to authenticate with the given apiKey and apiSecret
func (p *Peer) Logon(apiKey, apiSecret, bfxUserID string, cancelOnDisconnect bool) error {
	p.Rest.Credentials(apiKey, apiSecret)
	p.Ws.Credentials(apiKey, apiSecret)
	if cancelOnDisconnect {
		p.Ws.CancelOnDisconnect(true)
	}
	p.bfxUserID = bfxUserID
	log.Printf("peer connect %p", p.Ws)
	err := p.Ws.Connect()
	if err != nil {
		return err
	}
	go p.listen()
	return nil
}

func (p *Peer) listen() {
	p.started = true
	for msg := range p.Ws.Listen() {
		p.toParent <- &Message{Data: msg, Peer: p}
	}
	p.disconnect <- true
	close(p.exit)
}

// BfxUserID is an immutable accessor to the bitfinex user ID
func (p *Peer) BfxUserID() string {
	return p.bfxUserID
}

// FIXSessionID is an immutable accessor to the FIX session ID
func (p *Peer) FIXSessionID() quickfix.SessionID {
	return p.sessionID
}

// Close quits out of the associated websocket if started
func (p *Peer) Close() {
	if p.started {
		p.Ws.Close()
		<-p.exit
	}
	close(p.disconnect)
}
