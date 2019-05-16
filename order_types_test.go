package main

import (
	"fmt"
	"github.com/bitfinexcom/bfxfixgw/convert"
	"testing"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42ocrr "github.com/quickfixgo/fix42/ordercancelreplacerequest"
	"github.com/shopspring/decimal"
)

//TestNewOrderSingleGTD tests a custom expiration date when submitting a new order
func TestNewOrderSingleGTD(t *testing.T) {
	set := mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		if err := srvWs.Stop(); err != nil {
			t.Fatal(err)
		}
	}()

	// assert FIX MD logon
	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// assert FIX order logon
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// assume both ws clients connected in setup()
	srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := srvWs.WaitForMessage(MarketDataClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	// assert order ws auth request
	msg, err = srvWs.WaitForMessage(OrdersClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// broadcast auth ack to both clients
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// parse test expire time
	expiration, err := time.Parse(convert.TimeInForceFormat, "2006-01-02 15:04:05")
	if err != nil {
		t.Fatal(err)
	}

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	nos.Set(field.NewTimeInForce(enum.TimeInForce_GOOD_TILL_DATE))
	nos.Set(field.NewExpireTime(expiration))
	session := fixOrd.LastSession()
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000","tif":"2006-01-02 15:04:05"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// get milliseconds for test timestamp
	expirationMilliStr := fmt.Sprintf("%d", expiration.UnixNano()/1000000)

	// service publish pending new ack
	srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,`+expirationMilliStr+`,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new ack, assert NEW
	srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,`+expirationMilliStr+`,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00", "59=6", "126=20060102-15:04:05.000")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,12000,"EXCHANGE LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 3)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=12000", "14=0.2168", "59=6", "126=20060102-15:04:05.000")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.78320284,12000,"EXCHANGE LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 4)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.7832", "39=2", "54=1", "55=tBTCUSD", "150=2", "151=0.0000", "6=12000", "14=1.000", "59=6", "126=20060102-15:04:05.000")
	if err != nil {
		t.Fatal(err)
	}
}

//TestNewOrderSingleRejectBadGTD rejects a custom expiration date when field is missing
func TestNewOrderSingleRejectBadGTD(t *testing.T) {
	set := mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		if err := srvWs.Stop(); err != nil {
			t.Fatal(err)
		}
	}()

	// assert FIX MD logon
	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// assert FIX order logon
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// assume both ws clients connected in setup()
	srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := srvWs.WaitForMessage(MarketDataClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	// assert order ws auth request
	msg, err = srvWs.WaitForMessage(OrdersClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// broadcast auth ack to both clients
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("tBTCUSD"),
		field.NewSide(enum.Side_SELL),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(14000.0), 1))
	nos.Set(field.NewTimeInForce(enum.TimeInForce_GOOD_TILL_DATE))
	session := fixOrd.LastSession()
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}

	// assert FIX message reject
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=j", "49=BFXFIX", "56=EXORG_ORD", "372=D", "380=5", "58=Conditionally Required Field Missing (126)")
	if err != nil {
		t.Fatal(err)
	}
}

//TestNewOrderSingleMargin tests margin order types
func TestNewOrderSingleMargin(t *testing.T) {
	set := mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		if err := srvWs.Stop(); err != nil {
			t.Fatal(err)
		}
	}()

	// assert FIX MD logon
	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// assert FIX order logon
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// assume both ws clients connected in setup()
	srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := srvWs.WaitForMessage(MarketDataClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	// assert order ws auth request
	msg, err = srvWs.WaitForMessage(OrdersClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// broadcast auth ack to both clients
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	nos.Set(field.NewCashMargin(enum.CashMargin_MARGIN_OPEN))
	session := fixOrd.LastSession()
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"MARGIN LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// service publish pending new ack
	srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"MARGIN LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new ack, assert NEW
	srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"MARGIN LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00", "544=3")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,12000,"MARGIN LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 3)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=12000", "14=0.2168", "544=3")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.78320284,12000,"MARGIN LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 4)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.7832", "39=2", "54=1", "55=tBTCUSD", "150=2", "151=0.0000", "6=12000", "14=1.000", "544=3")
	if err != nil {
		t.Fatal(err)
	}
}

//TestNewOrderSingleThenUpdate assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle, and can update it later
func TestNewOrderSingleThenUpdate(t *testing.T) {
	set := mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		if err := srvWs.Stop(); err != nil {
			t.Fatal(err)
		}
	}()

	// assert FIX MD logon
	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// assert FIX order logon
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// assume both ws clients connected in setup()
	srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := srvWs.WaitForMessage(MarketDataClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	// assert order ws auth request
	msg, err = srvWs.WaitForMessage(OrdersClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// broadcast auth ack to both clients
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	session := fixOrd.LastSession()
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// service publish pending new ack
	srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new ack, assert NEW
	srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	if err != nil {
		t.Fatal(err)
	}

	// send order update and change a bunch of fields
	oup := fix42ocrr.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("567"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	oup.Set(field.NewOrderQty(decimal.NewFromFloat(2.0), 1))
	oup.Set(field.NewPrice(decimal.NewFromFloat(21000.0), 1))
	if err := session.Send(oup); err != nil {
		t.Fatal(err)
	}

	// assert OrderUpdate
	msg, err = srvWs.WaitForMessage(OrdersClient, 2)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"ou",null,{"id":1234567,"price":"21000","amount":"2"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// parse test expire time, send another update, requires cache to have updated in order to work
	expiration, err := time.Parse(convert.TimeInForceFormat, "2006-01-02 15:04:05")
	if err != nil {
		t.Fatal(err)
	}
	oup.Set(field.NewOrigClOrdID("567"))
	oup.Set(field.NewClOrdID("678"))
	oup.Set(field.NewTimeInForce(enum.TimeInForce_GOOD_TILL_DATE))
	oup.Set(field.NewExpireTime(expiration))
	if err := session.Send(oup); err != nil {
		t.Fatal(err)
	}

	// assert OrderUpdate
	msg, err = srvWs.WaitForMessage(OrdersClient, 3)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"ou",null,{"id":1234567,"price":"21000","amount":"2","tif":"2006-01-02 15:04:05"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}
}

//TestNewOrderSingleOCO assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle with an OCO contingency
func TestNewOrderSingleOCO(t *testing.T) {
	set := mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		if err := srvWs.Stop(); err != nil {
			t.Fatal(err)
		}
	}()

	// assert FIX MD logon
	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// assert FIX order logon
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// assume both ws clients connected in setup()
	srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := srvWs.WaitForMessage(MarketDataClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	// assert order ws auth request
	msg, err = srvWs.WaitForMessage(OrdersClient, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// broadcast auth ack to both clients
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	nos.Set(field.NewContingencyType(enum.ContingencyType_ONE_CANCELS_THE_OTHER))
	session := fixOrd.LastSession()
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}

	// assert FIX message reject - need that stop price
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=j", "49=BFXFIX", "56=EXORG_ORD", "372=D", "380=5", "58=Conditionally Required Field Missing (99)")
	if err != nil {
		t.Fatal(err)
	}

	// put in stop price then assert OrderNew
	nos.Set(field.NewStopPx(decimal.NewFromFloat(11500.0), 1))
	if err := session.Send(nos); err != nil {
		t.Fatal(err)
	}
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000","price_oco_stop":"11500","flags":`+fmt.Sprint(convert.FlagOCO)+`}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}
}
