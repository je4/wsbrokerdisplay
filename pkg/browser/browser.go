package browser

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// Browser provides an automated headless/display browser controller based on Chrome DevTools Protocol (CDP).
type Browser struct {
	config           Config
	logger           zLogger.ZLogger
	tempDir          string
	allocCtx         context.Context
	allocCancel      context.CancelFunc
	taskCtx          context.Context
	taskCancel       context.CancelFunc
	closing          bool
	supervisorCancel context.CancelFunc
	mu               sync.RWMutex
	handlerMu        sync.RWMutex
	onConsole        ConsoleHandler
}

// Ensure Browser implements io.Closer.
var _ io.Closer = (*Browser)(nil)

// New creates a new Browser instance with the provided configuration and logger.
func New(conf Config, logger zLogger.ZLogger) *Browser {
	return &Browser{
		config: conf,
		logger: logger,
	}
}

// normalizeAddress ensures the remote address string has an appropriate scheme for CDP connections.
func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "ws://") || strings.HasPrefix(addr, "wss://") ||
		strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "ws://" + addr
}

// Start launches the Chrome/Chromium browser process or connects to a remote CDP endpoint.
func (b *Browser) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closing = false

	if b.taskCtx != nil && b.taskCtx.Err() == nil {
		return nil // already running
	}

	b.cleanupUnsafe()

	if b.config.Address != "" {
		return b.startRemoteUnsafe(ctx)
	}

	b.startSupervisorUnsafe()
	if err := b.startLocalUnsafe(ctx); err != nil {
		return err
	}
	return nil
}

func (b *Browser) startRemoteUnsafe(ctx context.Context) error {
	remoteURL := normalizeAddress(b.config.Address)
	b.allocCtx, b.allocCancel = chromedp.NewRemoteAllocator(context.Background(), remoteURL)
	b.taskCtx, b.taskCancel = chromedp.NewContext(b.allocCtx)
	b.setupTargetListenerUnsafe()

	if err := chromedp.Run(b.taskCtx); err != nil {
		b.cleanupUnsafe()
		return errors.Wrapf(err, "cannot connect to remote chrome at '%s'", remoteURL)
	}

	if b.logger != nil {
		b.logger.Info().Str("address", remoteURL).Msg("connected to remote chrome instance successfully")
	}
	return nil
}

func (b *Browser) startLocalUnsafe(ctx context.Context) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),

		//chromedp.DisableGPU,
		//chromedp.NoSandbox,
	)

	if b.config.ExecPath != "" {
		opts = append(opts, chromedp.ExecPath(b.config.ExecPath))
	}

	if b.config.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	if b.config.Kiosk {
		opts = append(opts, chromedp.Flag("kiosk", true))
	}

	if b.config.IgnoreCertErrors {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors", true))
	}

	if b.config.WindowWidth > 0 && b.config.WindowHeight > 0 {
		opts = append(opts, chromedp.WindowSize(b.config.WindowWidth, b.config.WindowHeight))
	}

	userDataDir := b.config.UserDataDir
	if userDataDir == "" {
		tmp, err := os.MkdirTemp("", "wsbrokerdisplay-*")
		if err != nil {
			return errors.Wrap(err, "cannot create temporary user-data-dir")
		}
		b.tempDir = tmp
		userDataDir = tmp
	}
	opts = append(opts, chromedp.UserDataDir(userDataDir))

	for _, flag := range b.config.CustomFlags {
		opts = append(opts, chromedp.Flag(flag, true))
	}

	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		if b.logger != nil {
			b.logger.Debug().Str("cmd", cmd.String()).Msgf("chrome launch command: %s", cmd.String())
		}
	}))

	b.allocCtx, b.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	b.taskCtx, b.taskCancel = chromedp.NewContext(b.allocCtx)
	b.setupTargetListenerUnsafe()

	if err := chromedp.Run(b.taskCtx); err != nil {
		b.cleanupUnsafe()
		return errors.Wrap(err, "cannot launch chrome browser instance")
	}

	if b.logger != nil {
		b.logger.Info().Msg("browser instance started successfully")
	}
	return nil
}

func (b *Browser) setupTargetListenerUnsafe() {
	chromedp.ListenTarget(b.taskCtx, func(ev any) {
		if consoleEv, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			var text strings.Builder
			for idx, arg := range consoleEv.Args {
				if idx > 0 {
					text.WriteString(" ")
				}
				if arg.Value != nil {
					text.WriteString(fmt.Sprintf("%v", arg.Value))
				} else if arg.Description != "" {
					text.WriteString(arg.Description)
				}
			}
			msg := ConsoleMessage{
				Level:     string(consoleEv.Type),
				Message:   text.String(),
				Timestamp: consoleEv.Timestamp.Time(),
			}

			b.handlerMu.RLock()
			handler := b.onConsole
			b.handlerMu.RUnlock()

			if handler != nil {
				handler(msg)
			}
		}
	})
}

func (b *Browser) startSupervisorUnsafe() {
	if b.supervisorCancel != nil {
		b.supervisorCancel()
	}
	var supCtx context.Context
	supCtx, b.supervisorCancel = context.WithCancel(context.Background())
	go b.supervisorLoop(supCtx)
}

func (b *Browser) supervisorLoop(supCtx context.Context) {
	for {
		b.mu.RLock()
		allocCtx := b.allocCtx
		taskCtx := b.taskCtx
		closing := b.closing
		b.mu.RUnlock()

		if closing {
			return
		}

		if allocCtx != nil && taskCtx != nil {
			select {
			case <-supCtx.Done():
				return
			case <-allocCtx.Done():
			case <-taskCtx.Done():
			}

			b.mu.Lock()
			if b.closing || supCtx.Err() != nil {
				b.mu.Unlock()
				return
			}

			if b.logger != nil {
				b.logger.Warn().Msg("browser instance terminated unexpectedly, attempting automatic restart...")
			}
			b.cleanupUnsafe()
			b.mu.Unlock()
		}

		select {
		case <-supCtx.Done():
			return
		case <-time.After(1 * time.Second):
		}

		b.mu.Lock()
		if b.closing || supCtx.Err() != nil {
			b.mu.Unlock()
			return
		}

		if b.taskCtx != nil && b.taskCtx.Err() == nil {
			b.mu.Unlock()
			continue
		}

		if err := b.startLocalUnsafe(context.Background()); err != nil {
			if b.logger != nil {
				b.logger.Error().Err(err).Msg("failed to restart browser instance, retrying later")
			}
			b.mu.Unlock()
			continue
		}
		b.mu.Unlock()

		if b.logger != nil {
			b.logger.Info().Msg("browser instance automatically restarted successfully")
		}
	}
}

// IsRunning reports whether the browser process is currently active and accepting commands.
func (b *Browser) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.taskCtx != nil && b.taskCtx.Err() == nil
}

// actionContext derives a child context from b.taskCtx that inherits cancellation/deadline from the caller's ctx.
func (b *Browser) actionContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.taskCtx == nil || b.taskCtx.Err() != nil {
		return nil, nil, errors.New("browser is not running")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var childCtx context.Context
	var cancel context.CancelFunc

	if deadline, ok := ctx.Deadline(); ok {
		childCtx, cancel = context.WithDeadline(b.taskCtx, deadline)
	} else if b.config.Timeout > 0 {
		childCtx, cancel = context.WithTimeout(b.taskCtx, b.config.Timeout)
	} else {
		childCtx, cancel = context.WithCancel(b.taskCtx)
	}

	stop := context.AfterFunc(ctx, func() {
		cancel()
	})

	return childCtx, func() {
		stop()
		cancel()
	}, nil
}

// Run executes custom chromedp actions within the browser context.
func (b *Browser) Run(ctx context.Context, actions ...chromedp.Action) error {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	if err := chromedp.Run(actCtx, actions...); err != nil {
		return errors.Wrap(err, "failed to execute browser actions")
	}
	return nil
}

// Close terminates the Chrome process, cancels context resources, and cleans up any temporary user-data directory.
func (b *Browser) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closing = true
	if b.supervisorCancel != nil {
		b.supervisorCancel()
		b.supervisorCancel = nil
	}

	b.cleanupUnsafe()
	if b.logger != nil {
		b.logger.Info().Msg("browser instance closed")
	}
	return nil
}

func (b *Browser) cleanupUnsafe() {
	if b.taskCancel != nil {
		b.taskCancel()
		b.taskCancel = nil
	}
	if b.allocCancel != nil {
		b.allocCancel()
		b.allocCancel = nil
	}
	if b.tempDir != "" {
		_ = os.RemoveAll(b.tempDir)
		b.tempDir = ""
	}
	b.taskCtx = nil
	b.allocCtx = nil
}
