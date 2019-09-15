package main

import (
	"bytes"
	"fmt"
	"github.com/bitfinexcom/bfxfixgw/integration_test/mock"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	"github.com/bitfinexcom/bitfinex-api-go/utils"
	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	MarketDataClient = 0
	OrdersClient     = 1
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

type mockFixSettings struct {
	APIKey     string
	APISecret  string
	BfxUserID  string
	FixVersion string
}

func runSuite(t *testing.T, fixVersion, fixBeginString string) {
	gwSuite := new(gatewaySuite)
	gwSuite.settings = mockFixSettings{
		APIKey:     "apiKey1",
		APISecret:  "apiSecret2",
		BfxUserID:  "user123",
		FixVersion: fixVersion,
	}
	gwSuite.fixVersionTag = fixBeginString
	suite.Run(t, gwSuite)
}

func TestGatewaySuiteFIX42(t *testing.T) {
	runSuite(t, "fix42", "FIX.4.2")
}

func TestGatewaySuiteFIX44(t *testing.T) {
	runSuite(t, "fix44", "FIX.4.4")
}

func TestGatewaySuiteFIX50(t *testing.T) {
	runSuite(t, "fix50", "FIXT.1.1")
}

type gatewaySuite struct {
	suite.Suite
	fixMd               *mock.TestFixClient
	fixOrd              *mock.TestFixClient
	srvWs               *mock.Ws
	gw                  *Gateway
	settings            mockFixSettings
	fixVersionTag       string
	isWsOnline          bool
	MarketDataSessionID string
	OrderSessionID      string
}

func (s *gatewaySuite) checkFixTags(fix string, tags ...string) (err error) {
	s.Require().Contains(fix, "8="+s.fixVersionTag)
	for _, t := range tags {
		if s.fixVersionTag != quickfix.BeginStringFIX42 && strings.HasPrefix(t, fmt.Sprintf("%d=", tag.ExecTransType)) {
			//ignore exec trans type for non-fix42
			continue
		} else if !s.Contains(fix, t) {
			if err == nil {
				err = fmt.Errorf("fix message %s does not contain", fix)
			}
			err = fmt.Errorf("%s %s", err.Error(), t)
		}
	}
	return
}

func (s *gatewaySuite) loadSettings(file string) *quickfix.Settings {
	cfg, err := os.Open(file)
	s.Require().Nil(err)

	settings, err := quickfix.ParseSettings(cfg)
	s.Require().Nil(err)

	return settings
}

func (s *gatewaySuite) SetupTest() {
	s.MarketDataSessionID = s.fixVersionTag + ":EXORG_MD->BFXFIX"
	s.OrderSessionID = s.fixVersionTag + ":EXORG_ORD->BFXFIX"
	if s.fixVersionTag == quickfix.BeginStringFIXT11 {
		appendStr := ":" + strings.ToUpper(s.settings.FixVersion)
		s.MarketDataSessionID += appendStr
		s.OrderSessionID += appendStr
	}

	// remove temporary directories
	tries := 40
	var err error
	for i := 0; i < tries; i++ {
		err = os.RemoveAll("tmp/")
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond * 250)
	}
	s.Require().Nil(err)

	// mock BFX websocket
	s.srvWs = mock.NewMockWs(6001)
	err = s.srvWs.Start()
	s.Require().Nil(err)

	params := websocket.NewDefaultParameters()
	params.URL = "ws://127.0.0.1:6001"
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
	gatewayMdSettings := s.loadSettings(fmt.Sprintf("conf/integration_test/service/marketdata_%s.cfg", s.settings.FixVersion))
	gatewayOrdSettings := s.loadSettings(fmt.Sprintf("conf/integration_test/service/orders_%s.cfg", s.settings.FixVersion))
	s.gw, err = New(gatewayMdSettings, gatewayOrdSettings, &factory, symbol.NewPassthroughSymbology())
	s.Require().Nil(err)
	err = s.gw.Start()
	s.Require().Nil(err)

	// mock FIX client
	clientMDSettings := s.loadSettings(fmt.Sprintf("conf/integration_test/client/marketdata_%s.cfg", s.settings.FixVersion))
	s.fixMd, err = mock.NewTestFixClient(clientMDSettings, fix.NewNoStoreFactory(), "MarketData")
	s.Require().Nil(err)
	s.fixMd.APIKey = s.settings.APIKey
	s.fixMd.APISecret = s.settings.APISecret
	s.fixMd.BfxUserID = s.settings.BfxUserID
	err = s.fixMd.Start()
	s.Require().Nil(err)
	if len(s.settings.BfxUserID) > 0 {
		err = s.srvWs.WaitForClientCount(1)
		s.Require().Nil(err)
	}

	clientOrdSettings := s.loadSettings(fmt.Sprintf("conf/integration_test/client/orders_%s.cfg", s.settings.FixVersion))
	s.fixOrd, err = mock.NewTestFixClient(clientOrdSettings, quickfix.NewFileStoreFactory(clientOrdSettings), "Orders")
	s.Require().Nil(err)
	s.fixOrd.APIKey = s.settings.APIKey
	s.fixOrd.APISecret = s.settings.APISecret
	s.fixOrd.BfxUserID = s.settings.BfxUserID
	err = s.fixOrd.Start()
	s.Require().Nil(err)
	if len(s.settings.BfxUserID) > 0 {
		err = s.srvWs.WaitForClientCount(2)
		s.Require().Nil(err)
	}

	s.isWsOnline = true
}

func (s *gatewaySuite) TearDownTest() {
	s.fixMd.Stop()
	s.fixOrd.Stop()
	s.gw.Stop()
	if s.isWsOnline {
		err := s.srvWs.Stop()
		s.Require().Nil(err)
		if err = s.srvWs.KillConnections(); err != nil {
			fmt.Printf("Killing connections error seen: %s", err.Error())
		}
		s.isWsOnline = false
	}
}
