package main

import (
	"fmt"
	"github.com/bitfinexcom/bfxfixgw/integration_test/mock"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"github.com/bitfinexcom/bitfinex-api-go/utils"
	"github.com/quickfixgo/quickfix"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"bytes"
	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"io/ioutil"
	"net/http"
)

type fixVersion string

const (
	Fix42 fixVersion = "fix42"
	Fix44 fixVersion = "fix44"
)

const (
	MarketDataClient           = 0
	OrdersClient               = 1
	MarketDataSessionID string = "FIX.4.2:EXORG_MD->BFXFIX"
	OrderSessionID      string = "FIX.4.2:EXORG_ORD->BFXFIX"
)

type testNonceFactory struct {
}

func (t *testNonceFactory) New() utils.NonceGenerator {
	return &mock.NonceGenerator{}
}

type testClientFactory struct {
	Params *websocket.Parameters
	Nonce  *testNonceFactory
	HTTPDo func(c *http.Client, req *http.Request) (*http.Response, error)
}

func (m *testClientFactory) NewWs() *websocket.Client {
	return websocket.NewWithParamsNonce(m.Params, m.Nonce.New())
}

func (m *testClientFactory) NewRest() *rest.Client {
	return rest.NewClientWithHttpDo(m.HTTPDo)
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
	APIKey, APISecret, BfxUserID string
	FixVersion                   fixVersion
}

func attemptRemove() error {
	tries := 40
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

func checkFixTags(fix string, tags ...string) error {
	for _, tag := range tags {
		if !strings.Contains(fix, tag) {
			return fmt.Errorf("FIX message %s does not contain %s", fix, tag)
		}
	}
	return nil
}

func setupWithClientCheck(t *testing.T, port int, settings mockFixSettings, checkClient bool) (*mock.TestFixClient, *mock.TestFixClient, *mock.Ws, *Gateway) {
	err := attemptRemove()
	if err != nil {
		t.Fatal(err)
	}

	// mock BFX websocket
	wsService := mock.NewMockWs(port)
	wsService.Start()
	params := websocket.NewDefaultParameters()
	params.URL = "ws://localhost:6001"
	params.AutoReconnect = true
	params.ReconnectAttempts = 5
	params.ReconnectInterval = time.Millisecond * 250 // 1.25s
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
		Nonce:  &testNonceFactory{},
		HTTPDo: httpDo,
	}
	// create gateway
	gatewayMdSettings := loadSettings(fmt.Sprintf("conf/integration_test/service/marketdata_%s.cfg", settings.FixVersion))
	gatewayOrdSettings := loadSettings(fmt.Sprintf("conf/integration_test/service/orders_%s.cfg", settings.FixVersion))
	gateway, err := New(gatewayMdSettings, gatewayOrdSettings, &factory, symbol.NewPassthroughSymbology())
	if err != nil {
		t.Fatal(err)
	}
	err = gateway.Start()
	if err != nil {
		t.Fatal(err)
	}

	// mock FIX client
	clientMDSettings := loadSettings(fmt.Sprintf("conf/integration_test/client/marketdata_%s.cfg", settings.FixVersion))
	clientMDFix, err := mock.NewTestFixClient(clientMDSettings, fix.NewNoStoreFactory(), "MarketData")
	if err != nil {
		t.Fatal(err)
	}
	clientMDFix.APIKey = settings.APIKey
	clientMDFix.APISecret = settings.APISecret
	clientMDFix.BfxUserID = settings.BfxUserID
	if err != nil {
		t.Fatalf("could not create FIX md client: %s", err.Error())
	}
	clientMDFix.Start()
	if checkClient {
		err = wsService.WaitForClientCount(1)
		if err != nil {
			t.Fatal(err)
		}
	}

	clientOrdSettings := loadSettings(fmt.Sprintf("conf/integration_test/client/orders_%s.cfg", settings.FixVersion))
	clientOrdFix, err := mock.NewTestFixClient(clientOrdSettings, quickfix.NewFileStoreFactory(clientOrdSettings), "Orders")
	clientOrdFix.APIKey = settings.APIKey
	clientOrdFix.APISecret = settings.APISecret
	clientOrdFix.BfxUserID = settings.BfxUserID
	if err != nil {
		t.Fatalf("could not create FIX ord client: %s", err.Error())
	}
	clientOrdFix.Start()
	if checkClient {
		err = wsService.WaitForClientCount(2)
		if err != nil {
			t.Fatal(err)
		}
	}

	if !checkClient {
		t.Log("omit client check")
	}

	return clientMDFix, clientOrdFix, wsService, gateway
}

func setup(t *testing.T, port int, settings mockFixSettings) (*mock.TestFixClient, *mock.TestFixClient, *mock.Ws, *Gateway) {
	return setupWithClientCheck(t, port, settings, true)
}

//TestLogon assures the gateway service will authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func TestLogon(t *testing.T) {
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
}

//TestLogonNoCredentials assures the gateway service will only authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func TestLogonNoCredentials(t *testing.T) {
	set := mockFixSettings{
		FixVersion: Fix42,
	}
	fixMd, fixOrd, srvWs, gw := setupWithClientCheck(t, 6001, set, false)
	defer func() {
		fixMd.Stop()
		fixOrd.Stop()
		gw.Stop()
		srvWs.Stop()
	}()

	// give clients an opportunity to connect, but they should NOT be established
	err := srvWs.WaitForClientCount(2)
	if err == nil {
		t.Fatal("expected no client connection (no credentials), but a client connection was erroneously established")
	}

	fix, err := fixMd.WaitForMessage(MarketDataSessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	// expect malformed logon
	err = checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	if err != nil {
		t.Fatal(err)
	}
	// logon reject prevents websocket connections
	err = srvWs.WaitForClientCount(0)
	if err != nil {
		t.Fatal(err)
	}

	// TODO assert reject?
}

func TestLogonInvalidCredentials(t *testing.T) {
	// TODO assert reject?
}
