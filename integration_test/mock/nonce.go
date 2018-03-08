package mock

import (
	"fmt"
)

type MockNonceGenerator struct {
	nonce string
	inc   int
}

func (m *MockNonceGenerator) Next(nonce string) {
	m.nonce = nonce
}

func (m *MockNonceGenerator) GetNonce() string {
	if m.nonce == "" {
		m.inc++
		return fmt.Sprintf("nonce%d", m.inc)
	}
	return m.nonce
}
