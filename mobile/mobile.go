package mobile

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/kianmhz/GooseRelayVPN/internal/carrier"
	"github.com/kianmhz/GooseRelayVPN/internal/config"
	"github.com/kianmhz/GooseRelayVPN/internal/session"
	"github.com/kianmhz/GooseRelayVPN/internal/socks"
	"github.com/xjasonlyu/tun2socks/v2/engine"
)

var (
	mu        sync.Mutex
	cancelFn  context.CancelFunc
	running   bool
	tunActive bool
)

func StartClient(configPath string, logPath string) error {
	mu.Lock()
	if running {
		mu.Unlock()
		return fmt.Errorf("client is already running")
	}
	mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()
	log.SetFlags(0)
	log.SetOutput(logFile)

	cfg, err := config.LoadClient(configPath)
	if err != nil {
		return err
	}

	carr, err := carrier.New(carrier.Config{
		ScriptURLs:  cfg.ScriptURLs,
		AESKeyHex:   cfg.AESKeyHex,
		DebugTiming: cfg.DebugTiming,
		Fronting: carrier.FrontingConfig{
			GoogleIP: cfg.GoogleIP,
			SNIHosts: cfg.SNIHosts,
		},
	})
	if err != nil {
		return fmt.Errorf("carrier init failed: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	cancelFn = cancel
	running = true
	mu.Unlock()

	go func() {
		if err := carr.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[client] carrier run error: %v", err)
		}
	}()

	factory := socks.SessionFactory(func(target string) *session.Session {
		return carr.NewSession(target)
	})

	err = socks.Serve(ctx, cfg.ListenAddr, factory)

	mu.Lock()
	running = false
	cancelFn = nil
	mu.Unlock()
	return err
}

func StopClient() {
	StopTun()
	mu.Lock()
	defer mu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}
}

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

func StartTun(fd int64, proxyAddr string) {
	key := &engine.Key{
		Proxy:  "socks5://" + proxyAddr,
		Device: fmt.Sprintf("fd://%d", fd),
		MTU:    1500,
	}
	engine.Insert(key)
	engine.Start()

	mu.Lock()
	tunActive = true
	mu.Unlock()
}

func StopTun() {
	mu.Lock()
	defer mu.Unlock()
	if tunActive {
		engine.Stop()
		tunActive = false
	}
}

