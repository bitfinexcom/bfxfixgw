package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"go.uber.org/zap"
)

type Websocket struct {
	peer.Peers
	logger *zap.Logger
}

func New(peers peer.Peers) *Websocket {
	return &Websocket{
		Peers:  peers,
		logger: log.Logger,
	}
}
