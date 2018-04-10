package symbol

import (
	"testing"
)

func TestFileSymbol(t *testing.T) {
	sym, err := NewFileSymbology("../../integration_test/example_symbol_master.txt")
	if err != nil {
		t.Fatal(err)
	}
	// test bfx -> counterparty symbols
	s, err := sym.FromBitfinex("tBTCUSD", "CounterpartyA")
	if err != nil {
		t.Fatal(err)
	}
	if "XBT" != s {
		t.Fatalf("expected XBT, got %s", s)
	}
	s, err = sym.FromBitfinex("tETHUSD", "CounterpartyA")
	if err != nil {
		t.Fatal(err)
	}
	if "XTH" != s {
		t.Fatalf("expected XTH, got %s", s)
	}
	s, err = sym.FromBitfinex("tBTCUSD", "CounterpartyB")
	if err != nil {
		t.Fatal(err)
	}
	if "BTC" != s {
		t.Fatalf("expected BTC, got %s", s)
	}
	s, err = sym.FromBitfinex("tETHUSD", "CounterpartyB")
	if err != nil {
		t.Fatal(err)
	}
	if "ETH" != s {
		t.Fatalf("expected ETH, got %s", s)
	}
	// test counterparty -> bfx symbols
	s, err = sym.ToBitfinex("XBT", "CounterpartyA")
	if err != nil {
		t.Fatal(err)
	}
	if "tBTCUSD" != s {
		t.Fatalf("expected tBTCUSD, got %s", s)
	}
	s, err = sym.ToBitfinex("XTH", "CounterpartyA")
	if err != nil {
		t.Fatal(err)
	}
	if "tETHUSD" != s {
		t.Fatalf("expected tETHUSD, got %s", s)
	}
	s, err = sym.ToBitfinex("BTC", "CounterpartyB")
	if err != nil {
		t.Fatal(err)
	}
	if "tBTCUSD" != s {
		t.Fatalf("expected tBTCUSD, got %s", s)
	}
	s, err = sym.ToBitfinex("ETH", "CounterpartyB")
	if err != nil {
		t.Fatal(err)
	}
	if "tETHUSD" != s {
		t.Fatalf("expected tETHUSD, got %s", s)
	}
}
