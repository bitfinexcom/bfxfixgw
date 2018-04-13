package symbol

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// index is the bitfinex symbol
type symbolset struct {
	symbols     map[string]string
	passthrough bool
}

func newSymbolset() *symbolset {
	return &symbolset{
		symbols: make(map[string]string),
	}
}

func (s *symbolset) set(k, v string) {
	s.symbols[k] = v
}

func (s *symbolset) get(k string) (string, bool) {
	sym, ok := s.symbols[k]
	return sym, ok
}

// FileSymbology parses a simple KVP symbology mapping.  Counterparty names are wrapped with [square brackets] and prefix a symbol mapping set.
// L-values are Bitfinex symbols, R-values are counterparty symbols.
// ex:
// [Bloomberg]
// tBTCUSD=BXY
type FileSymbology struct {
	counterparty   string
	counterparties map[string]*symbolset
	lock           sync.Mutex
}

func (f *FileSymbology) parse(line string) {
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		f.counterparty = line[1 : len(line)-1]
	}
	s := strings.Split(line, "=")
	if len(s) < 2 {
		return
	}
	symbols, ok := f.counterparties[f.counterparty]
	if !ok {
		symbols = newSymbolset()
		f.counterparties[f.counterparty] = symbols
	}
	if strings.ToLower(s[0]) == "passthrough" && strings.ToLower(s[1]) == "true" {
		symbols.passthrough = true
	} else {
		symbols.set(s[0], s[1])
	}
}

func NewFileSymbology(path string) (*FileSymbology, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := &FileSymbology{counterparties: make(map[string]*symbolset)}
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		s.parse(scanner.Text())
	}
	return s, nil
}

func (f *FileSymbology) ToBitfinex(symbol, counterparty string) (string, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	symset, ok := f.counterparties[counterparty]
	if !ok {
		return "", fmt.Errorf("could not find counterparty: %s", counterparty)
	}
	if symset.passthrough {
		return symbol, nil
	}
	for bfx, cp := range symset.symbols {
		if cp == symbol {
			return bfx, nil
		}
	}
	return "", fmt.Errorf("could not find Bitfinex symbol mapping \"%s\" for counterparty \"%s\"", symbol, counterparty)
}

func (f *FileSymbology) FromBitfinex(symbol, counterparty string) (string, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	symset, ok := f.counterparties[counterparty]
	if !ok {
		return "", fmt.Errorf("could not find counterparty: %s", counterparty)
	}
	if symset.passthrough {
		return symbol, nil
	}
	sym, ok := symset.get(symbol)
	if !ok {
		return "", fmt.Errorf("could not find symbol \"%s\" for counterparty \"%s\"", symbol, counterparty)
	}
	return sym, nil
}
