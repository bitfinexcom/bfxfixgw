package symbol

import (
	"testing"
)

func TestFileSymbol(t *testing.T) {
	sym, err := NewFileSymbology("../../integration_test/example_symbol_master.txt")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sym.FromBitfinex("tBTCUSD", "CounterpartyA")
	if err != nil {
		t.Fatal(err)
	}
	if "XBT" != s {
		t.Fatalf("expected XBT, got %s", s)
	}
}
