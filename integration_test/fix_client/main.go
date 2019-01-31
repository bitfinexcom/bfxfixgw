package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bitfinexcom/bfxfixgw/service/fix"

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

// standalone FIX client
func main() {
	cfg := flag.String("cfg", "conf/integration_test/client/orders_fix42.cfg", "config path")
	cod := flag.Bool("cod", true, "cancel on disconnect")
	flag.Parse()

	// setup mocks
	settings := loadSettings(*cfg)
	var storeFactory quickfix.MessageStoreFactory
	if strings.Contains(*cfg, "orders") {
		storeFactory = quickfix.NewFileStoreFactory(settings)
	} else {
		storeFactory = fix.NewNoStoreFactory()
	}
	client, err := mock.NewTestFixClient(settings, storeFactory, "Client")
	if err != nil {
		log.Fatal(err)
	}
	apiKey, err := settings.GlobalSettings().Setting("ApiKey")
	if err != nil {
		log.Fatal("Please provide an 'ApiKey' setting in your FIX global configuration settings")
	}
	apiSecret, err := settings.GlobalSettings().Setting("ApiSecret")
	if err != nil {
		log.Fatal("Please provide an 'ApiSecret' setting in your FIX global configuration settings")
	}
	bfxUser, err := settings.GlobalSettings().Setting("BfxUserID")
	if err != nil {
		log.Fatal("Please provide an 'BfxUserID' setting in your FIX global configuration settings")
	}
	control := newControl(client)
	control.cmds["nos"] = &cmd.Order{}
	control.cmds["md"] = &cmd.MarketData{}
	control.cmds["cxl"] = &cmd.Cancel{}
	client.MessageHandler = control
	client.APIKey = apiKey
	client.APISecret = apiSecret
	client.BfxUserID = bfxUser
	client.CancelOnDisconnect = *cod
	if err = client.Start(); err != nil {
		log.Fatal(err)
	}

	go control.run()

	c := make(chan os.Signal, 1)
	exit := make(chan int)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go listenSignal(c, exit)

	ex := <-exit
	client.Stop()
	os.Exit(ex)
}
