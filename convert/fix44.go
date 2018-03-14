package convert

import (
	"strconv"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	fix44er "github.com/quickfixgo/fix44/executionreport"
	fix44mdsfr "github.com/quickfixgo/fix44/marketdatasnapshotfullrefresh"
	//fix44nos "github.com/quickfixgo/quickfix/fix44/newordersingle"
)

// converts bitfinex messages to FIX44

func FIX44ExecutionReportFromOrder(o *bitfinex.Order, cumQty float64) fix44er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err != nil {
		execID = uid.String()
	}
	amt := decimal.NewFromFloat(cumQty)

	e := fix44er.New(
		field.NewOrderID(strconv.FormatInt(o.ID, 10)),
		field.NewExecID(execID), // XXX: Can we just take a random ID here?
		field.NewExecType(enum.ExecType_ORDER_STATUS),
		field.NewOrdStatus(OrdStatusToFIX(o.Status)),
		field.NewSide(SideToFIX(o.Amount)),
		LeavesQtyToFIX(o.Amount),
		field.NewCumQty(amt, 2),
		AvgPxToFIX(o.PriceAvg),
	)

	e.SetSymbol(o.Symbol)

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
