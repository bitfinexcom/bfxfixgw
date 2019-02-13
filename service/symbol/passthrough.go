package symbol

//PassthroughSymbology passes symbology through without converting it
type PassthroughSymbology struct{}

//NewPassthroughSymbology creates a new passthrough symbology object
func NewPassthroughSymbology() Symbology {
	return &PassthroughSymbology{}
}

//ToBitfinex passes symbology through without conversion
func (p *PassthroughSymbology) ToBitfinex(symbol, counterparty string) (string, error) {
	return symbol, nil
}

//FromBitfinex passes symbology through without conversion
func (p *PassthroughSymbology) FromBitfinex(symbol, counterparty string) (string, error) {
	return symbol, nil
}
