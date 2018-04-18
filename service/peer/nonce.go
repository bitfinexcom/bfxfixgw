package peer

import (
	"github.com/bitfinexcom/bitfinex-api-go/utils"
	"strconv"
	"sync"
	"time"
)

type MultikeyNonceGenerator struct {
	ts, nonce uint64
	m         sync.Mutex
}

// API key must be exlusively used in this process space.
// Atomic counter per-time.Now() update supporting up to 999 operations per tick
func (u *MultikeyNonceGenerator) GetNonce() string {
	u.m.Lock()
	var s string
	n := uint64(time.Now().Unix()) * 1000
	if u.ts == n {
		u.nonce++
	} else {
		u.ts = n
		u.nonce = 1
	}
	s = strconv.FormatUint(u.ts+u.nonce, 10)
	u.m.Unlock()
	return s
}

func NewMultikeyNonceGenerator() utils.NonceGenerator {
	return &MultikeyNonceGenerator{
		ts:    uint64(time.Now().Unix()) * 1000,
		nonce: 1,
	}
}

// v1 support
var ts uint64
var nonce uint64

func reset() {
	ts = uint64(time.Now().Unix()) * 1000
	nonce = 1
}

func init() {
	reset()
}
