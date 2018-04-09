package main

import (
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	mdr "github.com/quickfixgo/fix42/marketdatarequest"
	"strings"
	"testing"
)

func newMdRequest(reqID, symbol string, depth int) *mdr.MarketDataRequest {
	mdreq := mdr.New(field.NewMDReqID(reqID), field.NewSubscriptionRequestType(enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES), field.NewMarketDepth(depth))
	nrsg := mdr.NewNoRelatedSymRepeatingGroup()
	nrs := nrsg.Add()
	nrs.Set(field.NewSymbol(symbol))
	mdreq.SetNoRelatedSym(nrsg)
	return &mdreq
}

func TestMarketData(t *testing.T) {
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

	// assert FIX MD logon
	fix, err = fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	fix, err = fixOrd.WaitForMessage(OrderSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	if err != nil {
		t.Fatal(err)
	}

	// request market data
	req := newMdRequest("request-id-1", "tBTCUSD", 1)
	fixMd.Send(req)

	// wait for ws to see requests
	msg, err = srvWs.WaitForMessage(MarketDataClient, 1)
	if err != nil {
		t.Fatal(err)
	}
	// raw book precision, no frequency
	if `{"subId":"nonce2","event":"subscribe","channel":"book","symbol":"tBTCUSD","prec":"R0","len":"1"}` != msg {
		t.Fatalf("msg was not as expected, got: %s", msg)
	}
	msg, err = srvWs.WaitForMessage(MarketDataClient, 2)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce3","event":"subscribe","channel":"trades","symbol":"tBTCUSD"}` != msg {
		t.Fatalf("msg was not as expected, got: %s", msg)
	}

	// ack book sub req
	srvWs.Send(MarketDataClient, `{"event":"subscribed","channel":"book","chanId":8,"symbol":"tBTCUSD","prec":"P0","freq":"F0","len":"1","subId":"nonce1","pair":"BTCUSD"}`)

	// ack trades sub req
	srvWs.Send(MarketDataClient, `{"event":"subscribed","channel":"trades","chanId":19,"symbol":"tBTCUSD","subId":"nonce1","pair":"BTCUSD"}`)

	// srv->client book snapshot--crash
	srvWs.Send(MarketDataClient, `[8,[[1085.2,1,0.16337353],[1085,1,1],[1084.5,1,0.0360446]]]`)
	// srv->client book update
	srvWs.Send(MarketDataClient, `[8,[1084,1,0.05246595]]`)

	// TODO assert snapshots

	// srv->client trade snapshot
	srvWs.Send(MarketDataClient, `[19,[[24165028,1516316211920,-0.05955414,1085.2],[24165027,1516316200519,-0.04440374,1085.2],[24165026,1516316189651,-0.0551028,1085.2]]]`)
	// srv->client trade update
	srvWs.Send(MarketDataClient, `[19,[24165025,1516316086676,-0.05246595,1085.2]]`)

	fix, err = fixMd.WaitForMessage("FIXT.1.1:BBGBETA->BFNXBETA", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test got FIX: %s", fix)
	// MsgType (35) = X
	if !strings.Contains(fix, "35=X") {
		t.Fatal(fix)
	}
	// NoMDEntries (268) = 1
	if !strings.Contains(fix, "268=1") {
		t.Fatal(fix)
	}
	// MdUpdateAction (279) = New (0)
	if !strings.Contains(fix, "279=0") {
		t.Fatal(fix)
	}
	// MDEntryType (269) = Ask (0)
	if !strings.Contains(fix, "269=0") {
		t.Fatal(fix)
	}
	// MDStreamID (1500) = 1
	if !strings.Contains(fix, "1500=1") {
		t.Fatal(fix)
	}
	// SecurityID (48) = BXY
	if !strings.Contains(fix, "48=BXY") {
		t.Fatal(fix)
	}
	// SecurityIDSource (22) = Exchange Symbol (8)
	if !strings.Contains(fix, "22=8") {
		t.Fatal(fix)
	}
	// MDFeedType (1022) = BFNX
	if !strings.Contains(fix, "1022=BFNX") {
		t.Fatal(fix)
	}
	// MDEntrySize (271) = 5
	if !strings.Contains(fix, "271=5") {
		t.Fatal(fix)
	}
	fix, err = fixMd.WaitForMessage("FIXT.1.1:BBGBETA->BFNXBETA", 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test got FIX 3: %s", strings.Replace(fix, string(0x1), "|", -1))
	// MsgType (35) = X
	if !strings.Contains(fix, "35=X") {
		t.Fatal(fix)
	}
	// NoMDEntries (268) = 1
	if !strings.Contains(fix, "268=1") {
		t.Fatal(fix)
	}
	// MdUpdateAction (279) = New (0)
	if !strings.Contains(fix, "279=0") {
		t.Fatal(fix)
	}
	// MDEntryType (269) = Trade (2)
	if !strings.Contains(fix, "269=2") {
		t.Fatal(fix)
	}
	// MDStreamID (1500) = 1
	if !strings.Contains(fix, "1500=1") {
		t.Fatal(fix)
	}
	// SecurityID (48) = BXY
	if !strings.Contains(fix, "48=BXY") {
		t.Fatal(fix)
	}
	// SecurityIDSource (22) = Exchange Symbol (8)
	if !strings.Contains(fix, "22=8") {
		t.Fatal(fix)
	}
	// MDFeedType (1022) = BFNX
	if !strings.Contains(fix, "1022=BFNX") {
		t.Fatal(fix)
	}
	// MDEntrySize (271) = 5
	if !strings.Contains(fix, "271=5") {
		t.Fatal(fix)
	}
}
