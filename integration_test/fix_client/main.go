package main

import (
	"flag"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"log"
	"os"
	"os/signal"
	"strings"
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

// standalone FIX client
func main() {
	cfg := flag.String("cfg", "conf/integration_test/client/orders_fix42.cfg", "config path")
	key := flag.String("key", "U83q9jkML2GVj1fVxFJOAXQeDGaXIzeZ6PwNPQLEXt4", "API key")
	secret := flag.String("secret", "77SWIRggvw0rCOJUgk9GVcxbldjTxOJP5WLCjWBFIVc", "API Secret")
	bfxUser := flag.String("user", "connamara", "BFX user ID")
	flag.Parse()

	// setup mocks
	settings := loadSettings(*cfg)
	var storeFactory quickfix.MessageStoreFactory
	if strings.Contains(*cfg, "orders") {
		storeFactory = quickfix.NewFileStoreFactory(settings)
	} else {
		storeFactory = fix.NewNoStoreFactory()
	}
	client, err := mock.NewTestFixClient(settings, storeFactory)
	if err != nil {
		log.Fatal(err)
	}
	control := newControl(client)
	control.cmds["nos"] = &cmd.Order{}
	control.cmds["md"] = &cmd.MarketData{}
	client.MessageHandler = control
	client.ApiKey = *key
	client.ApiSecret = *secret
	client.BfxUserID = *bfxUser
	client.Start()

	go control.run()

	c := make(chan os.Signal, 1)
	exit := make(chan int)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go listenSignal(c, exit)

	ex := <-exit
	client.Stop()
	os.Exit(ex)
}
