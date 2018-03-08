package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/quickfixgo/quickfix"

	"github.com/bitfinexcom/bfxfixgw/integration_test/fix_client/cmd"
	"github.com/bitfinexcom/bfxfixgw/integration_test/mock"
)

func listenSignal(sig chan os.Signal, exit chan int) {
	for {
		s := <-sig
		switch s {
		case syscall.SIGINT:
			log.Print("SIGINT")
			exit <- 0
		case syscall.SIGTERM:
			log.Print("SIGTERM")
			exit <- 0
		default:
			log.Print("unknown signal")
			exit <- 1
		}
	}
}

// standalone mock gain FIX endpoint
func main() {
	cfg := flag.String("cfg", "integration_test/conf/mock_fix42_client.cfg", "config path")
	flag.Parse()

	//tracker := &spread{}

	// setup mocks
	mockGainSettings := loadSettings(*cfg)
	mockGainClient, err := mock.NewTestFixClient(mockGainSettings, quickfix.NewMemoryStoreFactory())
	if err != nil {
		log.Fatal(err)
	}
	control := newControl(mockGainClient)
	control.cmds["nos"] = &cmd.Order{}
	control.cmds["md"] = &cmd.MarketData{}
	mockGainClient.OmitLogMessages = true
	mockGainClient.MessageHandler = control
	//mockGainClient.SendOnLogon(buildFixMdRequests([]string{"BTCUSD"}))
	mockGainClient.Start()
	defer mockGainClient.Stop()

	go control.run()

	c := make(chan os.Signal, 1)
	exit := make(chan int)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go listenSignal(c, exit)

	os.Exit(<-exit)
}
