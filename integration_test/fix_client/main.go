package main

import (
	"flag"
	"github.com/bitfinexcom/bfx-fix-mkt-go/test"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	tracker := &spread{}

	// setup mocks
	mockGainSettings := loadSettings(*cfg)
	mockGainClient, err := test.NewTestFixClient(mockGainSettings)
	if err != nil {
		log.Fatal(err)
	}
	mockGainClient.OmitLogMessages = true
	mockGainClient.MessageHandler = tracker
	mockGainClient.SendOnLogon(buildFixRequests([]string{"BTCUSD"}))
	mockGainClient.Start()
	defer mockGainClient.Stop()

	c := make(chan os.Signal, 1)
	exit := make(chan int)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go listenSignal(c, exit)

	os.Exit(<-exit)
}
