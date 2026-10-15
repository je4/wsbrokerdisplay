package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	rawclient "github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerdisplay/pkg/browser"
	"github.com/je4/wsbrokerdisplay/pkg/display"
	"github.com/je4/wsbrokerdisplay/pkg/rest"
	"github.com/rs/zerolog"
)

var configfile = flag.String("config", "", "location of toml configuration file")

func main() {
	flag.Parse()

	// 1. Load application configuration (from file or embedded default)
	conf, err := LoadConfig(*configfile)
	if err != nil {
		log.Fatalf("cannot load configuration: %v", err)
	}

	// 2. Initialize structured logging
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	loggerZero := zerolog.New(output).With().Timestamp().Str("app", "wsbrokerdisplay").Logger()
	var logger zLogger.ZLogger = &loggerZero

	addr := conf.Addr
	if addr == "" {
		addr = "localhost:8080"
	}
	extAddr := conf.ExtAddr
	if extAddr == "" {
		extAddr = fmt.Sprintf("http://%s", addr)
	}

	// 3. Initialize REST controller
	ctrl, err := rest.NewController(addr, extAddr, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot initialize rest controller")
	}

	// 4. Initialize Chromium browser controller
	browserCtrl := browser.New(conf.Browser, logger)
	browserCtx, browserCancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := browserCtrl.Start(browserCtx); err != nil {
		logger.Warn().Err(err).Msg("could not initialize browser at startup")
	}
	browserCancel()

	// 5. Initialize WSBroker client
	rawCfg := rawclient.DefaultConfig()
	rawOpts := []rawclient.Option{
		rawclient.WithEndpoint(conf.WSBroker),
		rawclient.WithClientName(conf.Name),
		rawclient.WithAutoReconnect(true),
		rawclient.WithLogger(logger),
	}
	if len(conf.Channels) > 0 {
		rawOpts = append(rawOpts, rawclient.WithInitialChannels(conf.Channels...))
	}
	if conf.ClientTLS.Type != "" || conf.ClientTLS.File != nil || conf.ClientTLS.Vault != nil || conf.ClientTLS.Dev != nil || conf.ClientTLS.Env != nil || conf.ClientTLS.ACME != nil {
		rawOpts = append(rawOpts, rawclient.WithTLSCertLoader(&conf.ClientTLS))
	}

	rawClient, err := rawclient.New(rawCfg, rawOpts...)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot initialize wsbroker client")
	}

	// 6. Initialize and start display coordinator
	dispCtrl := display.NewController(rawClient, browserCtrl, logger)
	dispCtrl.Start()

	if err := rawClient.Connect(context.Background()); err != nil {
		logger.Warn().Err(err).Msg("failed to initiate wsbroker connection")
	}

	// 7. Start REST service in background
	var wg sync.WaitGroup
	ctrl.Start(&wg)

	fmt.Println("==================================================")
	fmt.Printf(" WSBroker Display Application Started\n")
	fmt.Printf(" Client:   %s\n", conf.Name)
	fmt.Printf(" Broker:   %s\n", conf.WSBroker)
	fmt.Printf(" REST API: %s (%s/swagger/index.html)\n", addr, extAddr)
	fmt.Println("==================================================")
	fmt.Println("Press Ctrl+C to terminate...")

	// 8. Await termination signals for graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done

	fmt.Println("\nShutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	dispCtrl.Stop()
	if err := rawClient.Close(); err != nil {
		logger.Warn().Err(err).Msg("error closing wsbroker client")
	}
	if err := browserCtrl.Close(); err != nil {
		logger.Warn().Err(err).Msg("error closing browser controller")
	}

	ctrl.GracefulStop()
	wg.Wait()

	fmt.Println("WSBroker Display stopped.")
}
