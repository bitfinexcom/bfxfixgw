package convert

import (
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
	"strings"
	"time"
)

const (
	//FlagHidden represents a hidden order flag
	FlagHidden int = 64
	//FlagClose represents a close order flag
	FlagClose = 512
	//FlagPostOnly represents a post only order flag
	FlagPostOnly = 4096
	//FlagOCO represents an OCO order flag
	FlagOCO = 16384
)

// OrdStatusToFIX converts generic FIX types.
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

// ExecTypeToFIX follows FIX 4.1+ rules on merging ExecTransType + ExecType fields into new ExecType enums.
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

// SideToFIX converts amount to FIX side
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

// LeavesQtyToFIX converts amount to FIX field
func LeavesQtyToFIX(amount float64) field.LeavesQtyField {
	d := decimal.NewFromFloat(amount)
	return field.NewLeavesQty(d, 4)
}

// LastSharesToFIX converts qty to FIX field
func LastSharesToFIX(qty float64) field.LastSharesField {
	d := decimal.NewFromFloat(qty)
	return field.NewLastShares(d, 4)
}

// CumQtyToFIX converts cum qty to FIX field
func CumQtyToFIX(cumQty float64) field.CumQtyField {
	return field.NewCumQty(decimal.NewFromFloat(cumQty), 2)
}

// AvgPxToFIX converts price average to FIX field
func AvgPxToFIX(priceAvg float64) field.AvgPxField {
	d := decimal.NewFromFloat(priceAvg)
	return field.NewAvgPx(d, 2)
}

// OrdTypeToFIX converts bitfinex order type to FIX order type
func OrdTypeToFIX(bfxOrdType bitfinex.OrderType) (ordType enum.OrdType, isMargin bool) {
	isMargin = strings.Contains(string(bfxOrdType), "MARGIN")
	switch strings.Replace(string(bfxOrdType), "MARGIN", "EXCHANGE", 1) {
	case bitfinex.OrderTypeExchangeLimit:
		fallthrough
	case bitfinex.OrderTypeLimit:
		ordType = enum.OrdType_LIMIT
	case bitfinex.OrderTypeExchangeMarket:
		fallthrough
	case bitfinex.OrderTypeMarket:
		ordType = enum.OrdType_MARKET
	case bitfinex.OrderTypeStop:
		fallthrough
	case bitfinex.OrderTypeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeStop:
		ordType = enum.OrdType_STOP
	case bitfinex.OrderTypeStopLimit:
		ordType = enum.OrdType_STOP_LIMIT
	case bitfinex.OrderTypeFOK:
		fallthrough
	case bitfinex.OrderTypeExchangeFOK:
		ordType = enum.OrdType_LIMIT
	default:
		ordType = enum.OrdType_MARKET
	}
	return
}

// BookActionToFIX converts bitfinex book action to FIX MD enum
func BookActionToFIX(action bitfinex.BookAction) enum.MDUpdateAction {
	switch action {
	case bitfinex.BookUpdateEntry:
		return enum.MDUpdateAction_NEW
	case bitfinex.BookRemoveEntry:
		return enum.MDUpdateAction_DELETE
	}
	return enum.MDUpdateAction_NEW
}

// MTSToTime converts bitfinex millisecond timestamp to go time.Time
func MTSToTime(mts int64) (time.Time, bool) {
	if mts > 0 {
		return time.Unix(0, mts*1000000), true
	}
	return time.Time{}, false
}

// TimeInForceToFIX converts bitfinex order type to FIX TimeInForce
func TimeInForceToFIX(ordtype bitfinex.OrderType, mtstif int64) (enum.TimeInForce, time.Time) {
	tif, ok := MTSToTime(mtstif)
	if ok {
		return enum.TimeInForce_GOOD_TILL_DATE, tif
	}
	switch ordtype {
	case bitfinex.OrderTypeFOK:
		fallthrough
	case bitfinex.OrderTypeExchangeFOK:
		return enum.TimeInForce_FILL_OR_KILL, tif
	}
	return enum.TimeInForce_GOOD_TILL_CANCEL, tif // GTC default
}

// ExecInstToFIX converts bitfinex order type with flags to FIX exec inst
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

// DisplayMethodToFIX converts flags into FIX display method
func DisplayMethodToFIX(flags int) (enum.DisplayMethod, bool) {
	if flags&bitfinex.OrderFlagHidden != 0 {
		return enum.DisplayMethod_UNDISCLOSED, true
	}
	return "", false
}
