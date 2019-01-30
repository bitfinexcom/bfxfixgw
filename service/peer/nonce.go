package peer

import (
	"github.com/bitfinexcom/bitfinex-api-go/utils"
	"strconv"
	"sync"
	"time"
)

var ts, nonce uint64
var m sync.Mutex

// MultikeyNonceGenerator generates a nonce for the process space
type MultikeyNonceGenerator struct {
}

// GetNonce makes a new nonce
// API key must be exclusively used in this process space.
// Atomic counter per-time.Now() update supporting up to 999 operations per tick
func (u *MultikeyNonceGenerator) GetNonce() string {
	m.Lock()
	n := uint64(time.Now().Unix()) * 1000
	if ts == n {
		nonce++
	} else {
		ts = n
		nonce = 1
	}
	s := strconv.FormatUint(ts+nonce, 10)
	m.Unlock()
	return s
}

// NewMultikeyNonceGenerator returns a new nonce generator
func NewMultikeyNonceGenerator() utils.NonceGenerator {
	return &MultikeyNonceGenerator{}
}

func init() {
	ts = uint64(time.Now().Unix()) * 1000
	nonce = 1
}
