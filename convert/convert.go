// Package convert has utils to convert FIX4.(2|4) messages to and bitfinex
// API responses.
package convert

import (
	//"errors"
	"strconv"
	"strings"

	"github.com/knarz/bitfinex-api-go"
	uuid "github.com/satori/go.uuid"

	"github.com/quickfixgo/quickfix/enum"
	"github.com/quickfixgo/quickfix/field"
	"github.com/shopspring/decimal"

	fix44er "github.com/quickfixgo/quickfix/fix44/executionreport"
	fix44mdsfr "github.com/quickfixgo/quickfix/fix44/marketdatasnapshotfullrefresh"
	//fix44nos "github.com/quickfixgo/quickfix/fix44/newordersingle"

	fix42er "github.com/quickfixgo/quickfix/fix42/executionreport"
	fix42mdsfr "github.com/quickfixgo/quickfix/fix42/marketdatasnapshotfullrefresh"
	//fix42nos "github.com/quickfixgo/quickfix/fix42/newordersingle"
)

func Int64OrZero(i interface{}) int64 {
	if r, ok := i.(int64); ok {
		return r
	}
	return 0
}

func Float64OrZero(i interface{}) float64 {
	if r, ok := i.(float64); ok {
		return r
	}
	return 0.0
}

func BoolOrFalse(i interface{}) bool {
	if r, ok := i.(bool); ok {
		return r
	}
	return false
}

func StringOrEmpty(i interface{}) string {
	if r, ok := i.(string); ok {
		return r
	}
	return ""
}

func OrdStatusFromOrder(o *bitfinex.Order) field.OrdStatusField {
	orig, err := strconv.ParseFloat(o.OriginalAmount, 64)
	if err != nil {
		orig = 0.0
	}
	//exec, err := strconv.ParseFloat(o.ExecutedAmount, 64)
	//if err != nil {
	//exec = 0.0
	//}
	rem, err := strconv.ParseFloat(o.RemainingAmount, 64)
	if err != nil {
		rem = 0.0
	}

	switch {
	default:
		return field.NewOrdStatus(enum.OrdStatus_NEW)
	case o.IsCanceled:
		return field.NewOrdStatus(enum.OrdStatus_CANCELED)
	case rem > 0.0 && rem < orig:
		return field.NewOrdStatus(enum.OrdStatus_PARTIALLY_FILLED)
	case rem == 0.0:
		return field.NewOrdStatus(enum.OrdStatus_FILLED)
	}
}

func SideFromOrder(o *bitfinex.Order) field.SideField {
	switch {
	case strings.ToLower(o.Side) == "buy":
		return field.NewSide(enum.Side_BUY)
	case strings.ToLower(o.Side) == "sell":
		return field.NewSide(enum.Side_SELL)
	default:
		return field.NewSide(enum.Side_UNDISCLOSED)
	}
}

func LeavesQtyFromOrder(o *bitfinex.Order) field.LeavesQtyField {
	d, err := decimal.NewFromString(o.RemainingAmount)
	if err != nil {
		return field.NewLeavesQty(decimal.Zero, 2)
	}

	return field.NewLeavesQty(d, 2)
}

func CumQtyFromOrder(o *bitfinex.Order) field.CumQtyField {
	d, err := decimal.NewFromString(o.ExecutedAmount)
	if err != nil {
		return field.NewCumQty(decimal.Zero, 2)
	}

	return field.NewCumQty(d, 2)
}

func AvgPxFromOrder(o *bitfinex.Order) field.AvgPxField {
	d, err := decimal.NewFromString(o.AvgExecutionPrice)
	if err != nil {
		return field.NewAvgPx(decimal.Zero, 2)
	}

	return field.NewAvgPx(d, 2)
}

func FIX44ExecutionReportFromOrder(o *bitfinex.Order) fix44er.ExecutionReport {
	e := fix44er.New(
		field.NewOrderID(strconv.Itoa(o.Id)),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		field.NewExecType(enum.ExecType_ORDER_STATUS),
		OrdStatusFromOrder(o),
		SideFromOrder(o),
		LeavesQtyFromOrder(o),
		CumQtyFromOrder(o),
		AvgPxFromOrder(o),
	)

	e.SetSymbol(o.Symbol)

	return e
}

func FIX42ExecutionReportFromOrder(o *bitfinex.Order) fix42er.ExecutionReport {
	e := fix42er.New(
		field.NewOrderID(strconv.Itoa(o.Id)),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		// XXX: this method is only used to status at the moment but these should
		// probably not be hardcoded.
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(enum.ExecType_ORDER_STATUS),

		OrdStatusFromOrder(o),
		field.NewSymbol(o.Symbol),
		SideFromOrder(o),
		LeavesQtyFromOrder(o),
		CumQtyFromOrder(o),
		AvgPxFromOrder(o),
	)

	return e
}

func FIX44NoMDEntriesRepeatingGroupFromTradeTicker(data []float64) fix44mdsfr.NoMDEntriesRepeatingGroup {
	mdEntriesGroup := fix44mdsfr.NewNoMDEntriesRepeatingGroup()

	mde := mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_BID)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[0]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[1]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_OFFER)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[2]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[3]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[6]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE_VOLUME)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[7]), 8)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_HIGH_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[8]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_LOW_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[9]), 2)

	return mdEntriesGroup
}

func FIX42NoMDEntriesRepeatingGroupFromTradeTicker(data []float64) fix42mdsfr.NoMDEntriesRepeatingGroup {
	mdEntriesGroup := fix42mdsfr.NewNoMDEntriesRepeatingGroup()

	mde := mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_BID)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[0]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[1]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_OFFER)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[2]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[3]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[6]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE_VOLUME)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[7]), 8)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_HIGH_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[8]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_LOW_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[9]), 2)

	return mdEntriesGroup
}
