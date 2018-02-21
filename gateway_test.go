package main

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/bitfinexcom/bfxfixgw/integration_test/mock"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	"github.com/shopspring/decimal"
)

func loadSettings(file string) *quickfix.Settings {
	cfg, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	settings, err := quickfix.ParseSettings(cfg)
	if err != nil {
		log.Fatal(err)
	}
	return settings
}

type mockFixSettings struct {
	ApiKey, ApiSecret, BfxUserID string
	FixVersion                   fixVersion
}

func setup(t *testing.T, port int, settings mockFixSettings) (*mock.MockFIX, *mock.MockWs, *Gateway, *defaultClientFactory) {
	// setup mocks
	mockFIXSettings := loadSettings(fmt.Sprintf("integration_test/conf/mock_%s_client.cfg", settings.FixVersion))
	mockFIX, err := mock.NewMockFIX(mockFIXSettings)
	if err != nil {
		t.Fatal(err)
	}
	mockFIX.ApiKey = settings.ApiKey
	mockFIX.ApiSecret = settings.ApiSecret
	mockFIX.BfxUserID = settings.BfxUserID
	if err != nil {
		t.Fatalf("could not create FIX service: %s", err.Error())
		t.Fail()
	}
	mockFIX.Start()
	wsService := mock.NewMockWs(port)
	wsService.Start()
	params := websocket.NewDefaultParameters()
	params.URL = fmt.Sprintf("ws://localhost:%d", port)
	factory := defaultClientFactory{
		Parameters:     params,
		NonceGenerator: &mock.IncrementingNonceGenerator{},
	}

	// create gateway
	gatewayFIXSettings := loadSettings(fmt.Sprintf("integration_test/conf/mock_%s_service.cfg", settings.FixVersion))
	gateway, ok := newGateway(gatewayFIXSettings, &factory)
	if !ok {
		t.Fail()
	}
	err = gateway.Start()
	if err != nil {
		t.Fatal(err)
	}

	return mockFIX, wsService, gateway, &factory
}

type fixVersion string

const (
	Fix42 fixVersion = "fix42"
	Fix44 fixVersion = "fix44"
)

//TestLogon assures the gateway service will authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func TestLogon(t *testing.T) {
	set := mockFixSettings{
		ApiKey:     "apiKey1",
		ApiSecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	mockFIX, mockWs, gw, _ := setup(t, 6001, set)

	err := mockWs.WaitForClientCount(1)
	if err != nil {
		t.Fatal(err)
	}
	mockWs.Broadcast(`{"event":"info","version":2}`)

	msg, err := mockWs.WaitForMessage(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	mockFIX.Stop()
	mockWs.Stop()
	gw.Stop()
}

//TestLogonNoCredentials assures the gateway service will only authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func TestLogonNoCredentials(t *testing.T) {
	set := mockFixSettings{
		FixVersion: Fix42,
	}
	mockFIX, mockWs, gw, _ := setup(t, 6001, set)

	// give clients an opportunity to connect
	err := mockWs.WaitForClientCount(1)
	if err == nil {
		t.Fatal("expected no client connection (no credentials), but a client connection was erroneously established")
	}

	mockFIX.Stop()
	mockWs.Stop()
	gw.Stop()
}

//TestNewOrderSingle assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle
func TestNewOrderSingle(t *testing.T) {
	set := mockFixSettings{
		ApiKey:     "apiKey1",
		ApiSecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	mockFIX, mockWs, gw, _ := setup(t, 6001, set)

	err := mockWs.WaitForClientCount(1)
	if err != nil {
		t.Fatal(err)
	}
	mockWs.Broadcast(`{"event":"info","version":2}`)

	msg, err := mockWs.WaitForMessage(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got for logon: %s", msg)
	}
	// publish sub ack
	// 2018/02/02 17:01:22 [WARN]: invalid character 'c' looking for beginning of value ?
	mockWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS
	nos := fix42nos.New(field.NewClOrdID("clordid1"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	session := mockFIX.LastSession()
	session.Send(nos)

	// assert OrderNew
	msg, err = mockWs.WaitForMessage(0, 1)
	if `[0,"on",null,{"gid":0,"cid":0,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// ack new
	mockWs.Broadcast(`[0,"n",[null,"on-req",null,null,[1234567,null,clordid1,"tBTCUSD",null,null,1,1,"LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// position update
	mockWs.Broadcast(`[0,"pu",["tBTCUSD","ACTIVE",0.21679716,915.9,0,0,null,null,null,null]]`)
	mockWs.Broadcast(`[0,"pu",["tBTCUSD","ACTIVE",1,916.13496085,0,0,null,null,null,null]]`)

	// trade execution
	mockWs.Broadcast(`[0,"te",[1,"tBTCUSD",1514909325593,1234567,0.21679716,915.9,null,null,-1]]`)

	// TODO order update?

	mockFIX.Stop()
	mockWs.Stop()
	gw.Stop()
}
