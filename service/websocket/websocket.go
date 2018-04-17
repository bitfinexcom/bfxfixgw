package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"go.uber.org/zap"
)

type Websocket struct {
	peer.Peers
	symbol.Symbology
	logger *zap.Logger
}

func New(peers peer.Peers, symbology symbol.Symbology) *Websocket {
	return &Websocket{
		Peers:     peers,
		Symbology: symbology,
		logger:    log.Logger,
	}
}
