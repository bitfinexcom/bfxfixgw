package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service"
	"go.uber.org/zap"
)

type Websocket struct {
	service.Peers
	logger *zap.Logger
}

func NewWebsocket(lookup service.Peers) *Websocket {
	return &Websocket{
		Peers:  lookup,
		logger: log.Logger,
	}
}
