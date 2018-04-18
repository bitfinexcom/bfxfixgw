package convert

import (
	"errors"
	"strconv"

	"github.com/bitfinexcom/bitfinex-api-go/v2"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"

	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
)

// OrderNewTypeFromFIX42NewOrderSingle takes a FIX42 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) (string, error) {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	pref := "EXCHANGE "

	if ei == enum.ExecInst_ALL_OR_NONE {
		return "", errors.New("all or none execution instruction unsupported")
	}

	// map AON & IOC => FOK
	if tif == enum.TimeInForce_FILL_OR_KILL || ei == enum.ExecInst_ALL_OR_NONE && tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL {
		return pref + "FOK", nil
	}

	switch ot {
	case enum.OrdType_MARKET:
		return pref + "MARKET", nil
	case enum.OrdType_LIMIT:
		return pref + "LIMIT", nil
	case enum.OrdType_STOP:
		return pref + "STOP", nil
	case enum.OrdType_STOP_LIMIT:
		return "STOP LIMIT", nil
	default:
		return "", nil
	}
}

// OrderNewFromFIX42NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func OrderNewFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle, symbology symbol.Symbology, counterparty string) (*bitfinex.OrderNewRequest, quickfix.MessageRejectError) {
	on := &bitfinex.OrderNewRequest{}

	on.GID = 0
	cidstr, err := nos.GetClOrdID()
	if err != nil {
		return nil, err
	}
	cid, perr := strconv.ParseInt(cidstr, 10, 64)
	if perr != nil {
		cid = 0
	}
	on.CID = cid

	var er error
	on.Type, er = OrderNewTypeFromFIX42NewOrderSingle(nos)
	if er != nil {
		return nil, quickfix.NewMessageRejectError(er.Error(), 0, nil)
	}

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	translated, err2 := symbology.ToBitfinex(s, counterparty)
	var sym string
	if err2 == nil {
		sym = translated
	} else {
		sym = s
	}
	on.Symbol = sym

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	q, _ := qd.Float64()

	t, _ := nos.GetOrdType()
	switch t {
	case enum.OrdType_LIMIT:
		pd, err := nos.GetPrice()
		if err != nil {
			return nil, err
		}
		on.Price, _ = pd.Float64()
	case enum.OrdType_STOP:
		pd, err := nos.GetStopPx()
		if err != nil {
			return nil, err
		}
		on.Price, _ = pd.Float64()
	case enum.OrdType_STOP_LIMIT:
		lm, err := nos.GetPrice()
		if err != nil {
			return nil, err
		}
		on.Price, _ = lm.Float64()
		pd, err := nos.GetStopPx()
		if err != nil {
			return nil, err
		}
		on.PriceAuxLimit, _ = pd.Float64()
	}

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	if side == enum.Side_SELL {
		on.Amount = -q
	} else if side == enum.Side_BUY {
		on.Amount = q
	}

	return on, nil
}
