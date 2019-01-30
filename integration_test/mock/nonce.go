package mock

import (
	"fmt"
)

//NonceGenerator generates a mocked nonce generator
type NonceGenerator struct {
	nonce string
	inc   int
}

//Next assigns the nonce
func (m *NonceGenerator) Next(nonce string) {
	m.nonce = nonce
}

//GetNonce generates a new nonce
func (m *NonceGenerator) GetNonce() string {
	if m.nonce == "" {
		m.inc++
		return fmt.Sprintf("nonce%d", m.inc)
	}
	return m.nonce
}
