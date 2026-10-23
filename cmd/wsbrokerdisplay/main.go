package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	rawclient "github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/je4/wsbrokerclient/pkg/ntp"
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

	appStartTime := time.Now().UTC()

	// 3. Initialize WSBroker client
	rawCfg := rawclient.DefaultConfig()
	rawOpts := []rawclient.Option{
		rawclient.WithEndpoint(conf.WSBroker),
		rawclient.WithClientName(conf.Name),
		rawclient.WithAutoReconnect(true),
		rawclient.WithLogger(logger),
	}
	if conf.ReadLimit > 0 {
		rawOpts = append(rawOpts, rawclient.WithReadLimit(conf.ReadLimit))
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

	// 4. Initialize rolling WebSocket message log and register event listener
	maxMessages := conf.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 100
	}
	msgLog := rest.NewMessageLog(maxMessages)
	rawClient.OnEvent(func(evt *event.Event) {
		msgLog.AddEvent("in", evt)
	})

	// 5. Initialize and start NTP synchronization service via wsbrokerclient
	serverName := conf.NTP.Server
	if serverName == "" {
		serverName = "wsbroker"
	}
	ntpSvc := ntp.New(rawClient, serverName, conf.NTP.Interval, conf.NTP.Timeout, logger)
	ntpCtx, ntpCancel := context.WithCancel(context.Background())
	defer ntpCancel()
	ntpSvc.Start(ntpCtx)

	// 6. Configure default browser landing page URL
	if conf.Browser.GetStartURL() == "" {
		conf.Browser.StartURL = extAddr
	}

	// 7. Initialize REST controller with live telemetry data provider
	dataProvider := func() rest.DashboardData {
		hostname, _ := os.Hostname()
		ntpStatus := ntpSvc.Status()
		browserCfg := conf.Browser

		return rest.DashboardData{
			ClientName: conf.Name,
			WSBroker:   conf.WSBroker,
			Channels:   conf.Channels,
			Addr:       addr,
			ExtAddr:    extAddr,
			TLSType:    conf.ClientTLS.Type,
			Hostname:   hostname,
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
			GoVersion:  runtime.Version(),
			Uptime:     time.Since(appStartTime).Truncate(time.Second).String(),
			StartTime:  appStartTime,
			BrowserConfig: rest.BrowserInfo{
				Kiosk:    browserCfg.Kiosk,
				Headless: browserCfg.Headless,
				Width:    browserCfg.WindowWidth,
				Height:   browserCfg.WindowHeight,
				Address:  browserCfg.Address,
			},
			NTP:      ntpStatus,
			Messages: msgLog.GetAll(),
		}
	}

	ctrl, err := rest.NewController(addr, extAddr, logger, rest.WithDataProvider(dataProvider))
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot initialize rest controller")
	}

	// 8. Initialize Chromium browser controller
	browserCtrl := browser.New(conf.Browser, logger)
	browserCtx, browserCancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := browserCtrl.Start(browserCtx); err != nil {
		logger.Warn().Err(err).Msg("could not initialize browser at startup")
	}
	browserCancel()

	// 9. Initialize and start display coordinator with outgoing event logger
	dispCtrl := display.NewController(rawClient, browserCtrl, logger,
		display.WithOnSend(func(evt *event.Event) {
			msgLog.AddEvent("out", evt)
		}),
		display.WithStatusProvider(dataProvider),
	)
	dispCtrl.Start()

	if err := rawClient.Connect(context.Background()); err != nil {
		logger.Warn().Err(err).Msg("failed to initiate wsbroker connection")
	}

	// 9. Start REST service in background
	var wg sync.WaitGroup
	ctrl.Start(&wg)

	fmt.Println("==================================================")
	fmt.Printf(" WSBroker Display Application Started\n")
	fmt.Printf(" Client:   %s\n", conf.Name)
	fmt.Printf(" Broker:   %s\n", conf.WSBroker)
	fmt.Printf(" REST API: %s (%s/swagger/index.html)\n", addr, extAddr)
	fmt.Printf(" Default:  %s\n", conf.Browser.GetStartURL())
	fmt.Println("==================================================")
	fmt.Println("Press Ctrl+C to terminate...")

	// 10. Await termination signals for graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done

	fmt.Println("\nShutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	ntpSvc.Stop()
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
