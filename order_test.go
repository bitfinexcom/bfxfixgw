package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42cxl "github.com/quickfixgo/fix42/ordercancelrequest"
	"github.com/shopspring/decimal"
)

//TestNewOrderSingle assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle
func TestNewOrderSingle_BuyLimit_Fill(t *testing.T) {
	set := mockFixSettings{
		ApiKey:     "apiKey1",
		ApiSecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		srvWs.Stop()
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
	session.Send(nos)

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// service publish new ack
	srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// assert FIX execution report NEW
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,915.9,"MARKET",915.5,-1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 3)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=915.90", "14=0.2168")
	if err != nil {
		t.Fatal(err)
	}

	// trade execution
	srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.78320284,915.9,"MARKET",915.5,-1,-0.39712904,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 4)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.7832", "39=2", "54=1", "55=tBTCUSD", "150=2", "151=0.0000", "6=915.90", "14=1.000")
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrderCancelSimple(t *testing.T) {
	set := mockFixSettings{
		ApiKey:     "apiKey1",
		ApiSecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setup(t, 6001, set)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		srvWs.Stop()
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
	session.Send(nos)

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(OrdersClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// service publish new ack
	srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// assert FIX execution report NEW
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	if err != nil {
		t.Fatal(err)
	}

	// attempt to cancel order
	cxl := fix42cxl.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("556"),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()))
	session.Send(cxl)

	// assert cancel req
	msg, err = srvWs.WaitForMessage(OrdersClient, 2)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("YYYY-MM-dd")
	f := `[0,"oc",null,{"cid":8001,"cid_date":"%s"}]`
	if fmt.Sprintf(f, today) != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// publish cancel ack
	srvWs.Send(OrdersClient, `[0,"n",[1521062593962,"oc-req",null,null,[null,null,8001,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"SUCCESS","Submitted for cancellation; waiting for confirmation (ID: 555)."]]`)
	// TODO assert?

	// publish cancel success
	srvWs.Send(OrdersClient, `[0,"oc",[555,0,8001,"tBTCUSD",1521062529896,1521062593974,-1,-1,"EXCHANGE LIMIT",null,null,null,0,"CANCELED",null,null,15000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	// TODO assert?
}

func TestOrderCancelInFlightFill(t *testing.T) {
	// todo
}