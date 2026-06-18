// gozik-spotify is a backend music source agent for Gozik using the Spotify Web API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	"github.com/gg582/gozik-spotify/internal/desktop"
	"github.com/gg582/gozik-spotify/internal/notify"
	"github.com/gg582/gozik-spotify/internal/provider"
	"github.com/gg582/gozik-spotify/internal/webui"
	"google.golang.org/grpc"
)

const (
	defaultHost      = "127.0.0.1"
	defaultPort      = 50054 // intentionally not 50051 so it never collides with gozik-yt-music
	defaultWebUIPort = 50053 // intentionally not 50052 so it never collides with gozik-yt-music web UI
)

func main() {
	var (
		host              = flag.String("host", envOr("GOZIK_SPOTIFY_HOST", defaultHost), "gRPC bind host")
		port              = flag.Int("port", envIntOr("GOZIK_SPOTIFY_PORT", defaultPort), "gRPC bind port")
		webUIPort         = flag.Int("web-ui-port", envIntOr("GOZIK_SPOTIFY_WEBUI_PORT", defaultWebUIPort), "Web UI HTTP port (0 to disable)")
		registerDesktop   = flag.String("register-desktop-entry", envOr("GOZIK_SPOTIFY_REGISTER_DESKTOP", "auto"), "Desktop entry behaviour: auto/always/never")
		noStartupPopup    = flag.Bool("no-startup-popup", envBoolOr("GOZIK_SPOTIFY_NO_POPUP", false), "Disable the startup GUI popup")
	)
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}

	servicer, err := provider.New()
	if err != nil {
		log.Fatalf("create provider: %v", err)
	}

	s := grpc.NewServer(
		grpc.MaxSendMsgSize(64*1024*1024),
		grpc.MaxRecvMsgSize(64*1024*1024),
	)
	musicv1.RegisterMusicProviderServiceServer(s, servicer)

	// Start the optional standalone web UI so users can manage the plugin with
	// a browser and so plain HTTP tools like curl can reach the service.
	var webServer *webui.Server
	if *webUIPort > 0 {
		webServer = webui.New(servicer, *webUIPort)
		if err := webServer.Start(); err != nil {
			log.Fatalf("start web UI: %v", err)
		}

		if *registerDesktop != "never" {
			force := *registerDesktop == "always"
			if err := desktop.Register(*webUIPort, force); err != nil {
				log.Printf("desktop entry registration failed: %v", err)
			}
		}

		if !*noStartupPopup {
			go notify.Startup(*webUIPort)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down gRPC server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if webServer != nil {
			_ = webServer.Shutdown(ctx)
		}
		done := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			log.Println("graceful stop timed out; forcing immediate shutdown")
			s.Stop()
		}
	}()

	log.Printf("gozik-spotify listening on %s", lis.Addr().String())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}

func envBoolOr(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
