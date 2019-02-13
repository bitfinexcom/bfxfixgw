package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"go.uber.org/zap"
)

//Websocket is a session contained for peers, symbology, and logging
type Websocket struct {
	peer.Peers
	symbol.Symbology
	logger *zap.Logger
}

//New creates a new Websocket instance
func New(peers peer.Peers, symbology symbol.Symbology) *Websocket {
	return &Websocket{
		Peers:     peers,
		Symbology: symbology,
		logger:    log.Logger,
	}
}
