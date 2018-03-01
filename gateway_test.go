package main

import (
	"fmt"
	"github.com/bitfinexcom/bfxfixgw/integration_test/mock"
	"github.com/bitfinexcom/bfxfixgw/service"
	"github.com/quickfixgo/quickfix"
	"log"
	"os"
	"testing"
	"time"

	"bytes"
	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	"github.com/shopspring/decimal"
	"io/ioutil"
	"net/http"
)

type testClientFactory struct {
	Params *websocket.Parameters
	Nonce  *mock.MockNonceGenerator
	HttpDo func(c *http.Client, req *http.Request) (*http.Response, error)
}

func (m *testClientFactory) NewWs() *websocket.Client {
	return websocket.NewWithParamsNonce(m.Params, m.Nonce)
}

func (m *testClientFactory) NewRest() *rest.Client {
	return rest.NewClientWithHttpDo(m.HttpDo)
}

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

func attemptRemove() error {
	tries := 20
	var err error
	for i := 0; i < tries; i++ {
		err = os.RemoveAll("tmp/")
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond * 250)
	}
	return err
}

func setup(t *testing.T, port int, settings mockFixSettings) (*mock.TestFixClient, *mock.TestFixClient, *mock.MockWs, *Gateway, *testClientFactory) {
	log.Print("\n\n\n")
	err := attemptRemove()
	if err != nil {
		t.Fatal(err)
	}

	// mock BFX websocket
	wsService := mock.NewMockWs(port)
	wsService.Start()
	params := websocket.NewDefaultParameters()
	params.URL = "ws://localhost:6001"
	httpDo := func(_ *http.Client, req *http.Request) (*http.Response, error) {
		msg := "" // TODO http request handling (book snapshots?)
		resp := http.Response{
			Body:       ioutil.NopCloser(bytes.NewBufferString(msg)),
			StatusCode: 200,
		}
		return &resp, nil
	}
	factory := testClientFactory{
		Params: params,
		Nonce:  &mock.MockNonceGenerator{},
		HttpDo: httpDo,
	}
	// create gateway
	gatewayMdSettings := loadSettings(fmt.Sprintf("conf/integration_test/service/marketdata_%s.cfg", settings.FixVersion))
	gatewayOrdSettings := loadSettings(fmt.Sprintf("conf/integration_test/service/orders_%s.cfg", settings.FixVersion))
	gateway, err := New(gatewayMdSettings, gatewayOrdSettings, &factory)
	if err != nil {
		t.Fatal(err)
	}
	err = gateway.Start()
	if err != nil {
		t.Fatal(err)
	}

	// mock FIX client
	clientMDSettings := loadSettings(fmt.Sprintf("conf/integration_test/client/marketdata_%s.cfg", settings.FixVersion))
	clientMDFix, err := mock.NewTestFixClient(clientMDSettings, service.NewNoStoreFactory())
	if err != nil {
		t.Fatal(err)
	}
	clientMDFix.ApiKey = settings.ApiKey
	clientMDFix.ApiSecret = settings.ApiSecret
	clientMDFix.BfxUserID = settings.BfxUserID
	if err != nil {
		t.Fatalf("could not create FIX md client: %s", err.Error())
		t.Fail()
	}
	clientMDFix.Start()
	// ord service file store still in use:   (remove tmp/ord_service/data/FIX.4.2-BFXFIX-EXORG_ORD.body: The process cannot access the file because it is being used by another process.)

	clientOrdSettings := loadSettings(fmt.Sprintf("conf/integration_test/client/orders_%s.cfg", settings.FixVersion))
	clientOrdFix, err := mock.NewTestFixClient(clientOrdSettings, quickfix.NewFileStoreFactory(clientOrdSettings))
	clientOrdFix.ApiKey = settings.ApiKey
	clientOrdFix.ApiSecret = settings.ApiSecret
	clientOrdFix.BfxUserID = settings.BfxUserID
	if err != nil {
		t.Fatalf("could not create FIX md client: %s", err.Error())
		t.Fail()
	}
	clientOrdFix.Start()

	return clientMDFix, clientOrdFix, wsService, gateway, &factory
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
	srvMd, srvOrd, srvWs, gw, _ := setup(t, 6001, set)
	defer func() {
		srvMd.Stop()
		srvOrd.Stop()
		gw.Stop()
		srvWs.Stop()
	}()

	err := srvWs.WaitForClientCount(2)
	if err != nil {
		t.Fatal(err)
	}
	srvWs.Broadcast(`{"event":"info","version":2}`)

	msg, err := srvWs.WaitForMessage(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce2","event":"auth","apiKey":"apiKey1","authSig":"0c7c6fa4205423c1b0140000357bc9fd6cc114ee72370e8d6791e2dbe7a257abf6d6e42880cfc15b0db084ec6653052f","authPayload":"AUTHnonce2","authNonce":"nonce2"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}
	msg, err = srvWs.WaitForMessage(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got: %s", msg)
	}

	// TODO test for FIX logon
}

//TestLogonNoCredentials assures the gateway service will only authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func TestLogonNoCredentials(t *testing.T) {
	set := mockFixSettings{
		FixVersion: Fix42,
	}
	srvMd, srvOrd, srvWs, gw, _ := setup(t, 6001, set)
	defer func() {
		srvMd.Stop()
		srvOrd.Stop()
		gw.Stop()
		srvWs.Stop()
	}()

	// give clients an opportunity to connect, but they should NOT be established
	err := srvWs.WaitForClientCount(2)
	if err == nil {
		t.Fatal("expected no client connection (no credentials), but a client connection was erroneously established")
	}

	// TODO test for FIX reject
}

//TestNewOrderSingle assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle
func TestNewOrderSingle(t *testing.T) {
	set := mockFixSettings{
		ApiKey:     "apiKey1",
		ApiSecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: Fix42,
	}
	srvMd, srvOrd, srvWs, gw, _ := setup(t, 6001, set)
	defer func() {
		srvMd.Stop()
		srvOrd.Stop()
		gw.Stop()
		srvWs.Stop()
	}()

	err := srvWs.WaitForClientCount(2)
	if err != nil {
		t.Fatal(err)
	}
	srvWs.Broadcast(`{"event":"info","version":2}`)

	msg, err := srvWs.WaitForMessage(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}` != msg {
		t.Fatalf("unexpectedly got for logon: %s", msg)
	}
	msg, err = srvWs.WaitForMessage(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if `{"subId":"nonce2","event":"auth","apiKey":"apiKey1","authSig":"0c7c6fa4205423c1b0140000357bc9fd6cc114ee72370e8d6791e2dbe7a257abf6d6e42880cfc15b0db084ec6653052f","authPayload":"AUTHnonce2","authNonce":"nonce2"}` != msg {
		t.Fatalf("unexpectedly got for logon: %s", msg)
	}
	srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS
	nos := fix42nos.New(field.NewClOrdID("clordid1"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	session := srvOrd.LastSession()
	session.Send(nos)

	// assert OrderNew
	msg, err = srvWs.WaitForMessage(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if `[0,"on",null,{"gid":0,"cid":0,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]` != msg {
		t.Fatalf("unexpectedly got for order: %s", msg)
	}

	// ack new
	srvWs.Broadcast(`[0,"n",[null,"on-req",null,null,[1234567,null,clordid1,"tBTCUSD",null,null,1,1,"LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// position update
	srvWs.Broadcast(`[0,"pu",["tBTCUSD","ACTIVE",0.21679716,915.9,0,0,null,null,null,null]]`)
	srvWs.Broadcast(`[0,"pu",["tBTCUSD","ACTIVE",1,916.13496085,0,0,null,null,null,null]]`)

	// trade execution
	srvWs.Broadcast(`[0,"te",[1,"tBTCUSD",1514909325593,1234567,0.21679716,915.9,null,null,-1]]`)

	// TODO order update?
}
