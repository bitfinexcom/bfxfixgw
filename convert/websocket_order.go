package convert

import (
	"strconv"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	uuid "github.com/satori/go.uuid"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/quickfix"
	"github.com/shopspring/decimal"

	fix44er "github.com/quickfixgo/fix44/executionreport"
	//fix44mdsfr "github.com/quickfixgo/quickfix/fix44/marketdatasnapshotfullrefresh"
	fix44nos "github.com/quickfixgo/fix44/newordersingle"

	fix42er "github.com/quickfixgo/fix42/executionreport"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
)

func FIX44ExecutionReportRejectUnknown(oid, cid string) fix44er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err != nil {
		execID = uid.String()
	}
	e := fix44er.New(
		field.NewOrderID(oid),
		field.NewExecID(execID), // XXX: Can we just take a random ID here?
		field.NewExecType(enum.ExecType_ORDER_STATUS),
		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewSide(enum.Side_UNDISCLOSED),
		field.NewLeavesQty(decimal.NewFromFloat(0.0), 2),
		field.NewCumQty(decimal.NewFromFloat(0.0), 2),
		field.NewAvgPx(decimal.NewFromFloat(0.0), 2),
	)

	e.SetClOrdID(cid)
	e.SetOrdRejReason(enum.OrdRejReason_UNKNOWN_ORDER)

	return e
}

func FIX42ExecutionReportRejectUnknown(oid, cid string) fix42er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err != nil {
		execID = uid.String()
	}
	e := fix42er.New(
		field.NewOrderID(oid),
		field.NewExecID(execID), // XXX: Can we just take a random ID here?
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(enum.ExecType_ORDER_STATUS),

		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewSymbol(""),
		field.NewSide(enum.Side_UNDISCLOSED),
		field.NewLeavesQty(decimal.NewFromFloat(0.0), 2),
		field.NewCumQty(decimal.NewFromFloat(0.0), 2),
		field.NewAvgPx(decimal.NewFromFloat(0.0), 2),
	)

	e.SetClOrdID(cid)
	e.SetOrdRejReason(enum.OrdRejReason_UNKNOWN_ORDER)

	return e
}

// OrderNewTypeFromFIX44NewOrderSingle takes a FIX44 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX44NewOrderSingle(nos fix44nos.NewOrderSingle) string {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	pref := "EXCHANGE "

	switch {
	case ot == enum.OrdType_MARKET:
		return pref + "MARKET"
	case ot == enum.OrdType_LIMIT:
		return pref + "LIMIT"
	case ot == enum.OrdType_STOP:
		return pref + "STOP"
	case ot == enum.OrdType_STOP_LIMIT:
		return "STOP LIMIT"
	case tif == enum.TimeInForce_FILL_OR_KILL:
		return pref + "FOK"
	case ei == enum.ExecInst_ALL_OR_NONE && tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL:
		return pref + "FOK"
	default:
		return ""
	}
}

// OrderNewTypeFromFIX42NewOrderSingle takes a FIX42 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) string {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	pref := "EXCHANGE "

	switch {
	case ot == enum.OrdType_MARKET:
		return pref + "MARKET"
	case ot == enum.OrdType_LIMIT:
		return pref + "LIMIT"
	case ot == enum.OrdType_STOP:
		return pref + "STOP"
	case ot == enum.OrdType_STOP_LIMIT:
		return "STOP LIMIT"
	case tif == enum.TimeInForce_FILL_OR_KILL:
		return pref + "FOK"
	case ei == enum.ExecInst_ALL_OR_NONE && tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL:
		return pref + "FOK"
	default:
		return ""
	}
}

// OrderNewFromFIX44NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func OrderNewFromFIX44NewOrderSingle(nos fix44nos.NewOrderSingle) (*bitfinex.OrderNewRequest, quickfix.MessageRejectError) {
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

	on.Type = OrderNewTypeFromFIX44NewOrderSingle(nos)

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	on.Symbol = s

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	q, _ := qd.Float64()
	on.Amount = q

	pd, err := nos.GetPrice()
	if err != nil {
		return nil, err
	}
	p, _ := pd.Float64()

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	if side == enum.Side_SELL {
		on.Price = -p
	} else if side == enum.Side_BUY {
		on.Price = p
	}

	return on, nil
}

// OrderNewFromFIX42NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func OrderNewFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) (*bitfinex.OrderNewRequest, quickfix.MessageRejectError) {
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

	on.Type = OrderNewTypeFromFIX42NewOrderSingle(nos)

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	on.Symbol = s

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	q, _ := qd.Float64()
	on.Amount = q

	pd, err := nos.GetPrice()
	if err != nil {
		return nil, err
	}
	p, _ := pd.Float64()

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	if side == enum.Side_SELL {
		on.Price = -p
	} else if side == enum.Side_BUY {
		on.Price = p
	}

	return on, nil
}
