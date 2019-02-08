package convert

import (
	"errors"
	"strconv"
	"strings"

	"github.com/bitfinexcom/bitfinex-api-go/v2"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"

	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42ocrr "github.com/quickfixgo/fix42/ordercancelreplacerequest"
)

// OrderNewTypeFromFIX42NewOrderSingle takes a FIX42 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) (string, error) {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	if ei == enum.ExecInst_ALL_OR_NONE {
		return "", errors.New("all or none execution instruction unsupported")
	}

	// map AON & IOC => FOK
	if tif == enum.TimeInForce_FILL_OR_KILL || tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL {
		return bitfinex.OrderTypeExchangeFOK, nil
	}

	switch ot {
	case enum.OrdType_MARKET:
		return bitfinex.OrderTypeExchangeMarket, nil
	case enum.OrdType_LIMIT:
		return bitfinex.OrderTypeExchangeLimit, nil
	case enum.OrdType_STOP:
		execInst, err := nos.GetExecInst()
		if err == nil && strings.Contains(string(execInst), string(enum.ExecInst_PRIMARY_PEG)) {
			return bitfinex.OrderTypeExchangeTrailingStop, nil
		}
		return bitfinex.OrderTypeExchangeStop, nil
	case enum.OrdType_STOP_LIMIT:
		return bitfinex.OrderTypeExchangeStopLimit, nil
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

	// hidden
	displayMethod, err := nos.GetString(tag.DisplayMethod)
	if err == nil && enum.DisplayMethod(displayMethod) == enum.DisplayMethod_UNDISCLOSED {
		on.Hidden = true
	}
	execInst, err := nos.GetExecInst()
	// post only
	if err == nil && strings.Contains(string(execInst), string(enum.ExecInst_PARTICIPANT_DONT_INITIATE)) {
		on.PostOnly = true
	}
	// trailing stop
	if t == enum.OrdType_STOP {
		if err == nil && strings.Contains(string(execInst), string(enum.ExecInst_PRIMARY_PEG)) {
			trail, err := nos.GetPegDifference()
			if err != nil {
				return nil, err // trailing stop needs a peg
			}
			on.PriceTrailing, _ = trail.Float64()
		}
	}

	return on, nil
}

// OrderUpdateFromFIX42OrderCancelReplaceReques converts an OrderCancelReplaceRequest
// into an order update for the bitfinex websocket API, as best as it can.
func OrderUpdateFromFIX42OrderCancelReplaceRequest(ocrr fix42ocrr.OrderCancelReplaceRequest, symbology symbol.Symbology, counterparty string) (*bitfinex.OrderUpdateRequest, quickfix.MessageRejectError) {
	//TODO: implement conversion
	return nil, nil
}
