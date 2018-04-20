package convert

import (
	"strings"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
)

const (
	FlagHidden   int = 64
	FlagClose        = 512
	FlagPostOnly     = 4096
	FlagOCO          = 16384
)

// Generic FIX types.

func OrdStatusToFIX(status bitfinex.OrderStatus) enum.OrdStatus {
	// if the status is a composite (e.g. EXECUTED @ X: was PARTIALLY FILLED @ Y)
	// executed check must come first
	if strings.Contains(string(status), string(bitfinex.OrderStatusExecuted)) {
		return enum.OrdStatus_FILLED
	}
	if strings.Contains(string(status), string(bitfinex.OrderStatusPartiallyFilled)) {
		return enum.OrdStatus_PARTIALLY_FILLED
	}
	if strings.Contains(string(status), string(bitfinex.OrderStatusCanceled)) {
		return enum.OrdStatus_CANCELED
	}
	return enum.OrdStatus_NEW
}

// follows FIX 4.1+ rules on merging ExecTransType + ExecType fields into new ExecType enums.
func ExecTypeToFIX(status bitfinex.OrderStatus) enum.ExecType {
	if strings.Contains(string(status), string(bitfinex.OrderStatusActive)) {
		return enum.ExecType_NEW
	}
	if strings.Contains(string(status), string(bitfinex.OrderStatusCanceled)) {
		return enum.ExecType_CANCELED
	}
	if strings.Contains(string(status), string(bitfinex.OrderStatusPartiallyFilled)) {
		return enum.ExecType_TRADE
	}
	if strings.Contains(string(status), string(bitfinex.OrderStatusExecuted)) {
		return enum.ExecType_TRADE
	}
	return enum.ExecType_ORDER_STATUS
}

func SideToFIX(amount float64) enum.Side {
	switch {
	case amount > 0.0:
		return enum.Side_BUY
	case amount < 0.0:
		return enum.Side_SELL
	default:
		return enum.Side_UNDISCLOSED
	}
}

// qty
func LeavesQtyToFIX(amount float64) field.LeavesQtyField {
	d := decimal.NewFromFloat(amount)
	return field.NewLeavesQty(d, 4)
}

// qty
func LastSharesToFIX(qty float64) field.LastSharesField {
	d := decimal.NewFromFloat(qty)
	return field.NewLastShares(d, 4)
}

func CumQtyToFIX(cumQty float64) field.CumQtyField {
	return field.NewCumQty(decimal.NewFromFloat(cumQty), 2)
}

func AvgPxToFIX(priceAvg float64) field.AvgPxField {
	d := decimal.NewFromFloat(priceAvg)
	return field.NewAvgPx(d, 2)
}

func OrdTypeToFIX(ordtype bitfinex.OrderType) enum.OrdType {
	switch ordtype {
	case bitfinex.OrderTypeExchangeLimit:
		fallthrough
	case bitfinex.OrderTypeLimit:
		return enum.OrdType_LIMIT
	case bitfinex.OrderTypeExchangeMarket:
		fallthrough
	case bitfinex.OrderTypeMarket:
		return enum.OrdType_MARKET
	case bitfinex.OrderTypeStop:
		fallthrough
	case bitfinex.OrderTypeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeStop:
		return enum.OrdType_STOP
	case bitfinex.OrderTypeStopLimit:
		return enum.OrdType_STOP_LIMIT
	case bitfinex.OrderTypeFOK:
		fallthrough
	case bitfinex.OrderTypeExchangeFOK:
		return enum.OrdType_LIMIT
	}
	return enum.OrdType_MARKET
}

func BookActionToFIX(action bitfinex.BookAction) enum.MDUpdateAction {
	switch action {
	case bitfinex.BookUpdateEntry:
		return enum.MDUpdateAction_NEW
	case bitfinex.BookRemoveEntry:
		return enum.MDUpdateAction_DELETE
	}
	return enum.MDUpdateAction_NEW
}

func TimeInForceToFIX(ordtype bitfinex.OrderType) enum.TimeInForce {
	switch ordtype {
	case bitfinex.OrderTypeFOK:
		fallthrough
	case bitfinex.OrderTypeExchangeFOK:
		return enum.TimeInForce_FILL_OR_KILL
	}
	return enum.TimeInForce_GOOD_TILL_CANCEL // GTC default
}

func ExecInstToFIX(ordtype bitfinex.OrderType, flags int) (enum.ExecInst, bool) {
	execInst := ""
	switch ordtype {
	case bitfinex.OrderTypeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeTrailingStop:
		execInst = string(enum.ExecInst_PRIMARY_PEG)
	}
	if flags&bitfinex.OrderFlagPostOnly != 0 {
		execInst = execInst + string(enum.ExecInst_PARTICIPANT_DONT_INITIATE)
	}
	return enum.ExecInst(execInst), execInst != "" // helps determining if ExecInst should be set
}

func DisplayMethodToFIX(flags int) (enum.DisplayMethod, bool) {
	if flags&bitfinex.OrderFlagHidden != 0 {
		return enum.DisplayMethod_UNDISCLOSED, true
	}
	return "", false
}
