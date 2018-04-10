package symbol

// Symbology resolves counterparty symbols to & from Bitfinex API symbols
type Symbology interface {
	ToBitfinex(symbol, counterparty string) string
	FromBitfinex(symbol, counterparty string) string
}
