package symbol

type PassthroughSymbology struct{}

func NewPassthroughSymbology() Symbology {
	return &PassthroughSymbology{}
}

func (p *PassthroughSymbology) ToBitfinex(symbol, counterparty string) (string, error) {
	return symbol, nil
}

func (p *PassthroughSymbology) FromBitfinex(symbol, counterparty string) (string, error) {
	return symbol, nil
}
