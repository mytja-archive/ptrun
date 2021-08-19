package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"goji.io"
	"goji.io/pat"

	"github.com/ptr-geeks/ptrun/server/internal/ws"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	server ws.Server
)

var (
	metricsstate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ptrun_state",
		Help: "State",
	})
)

func main() {
	metricsstate.Set(0)

	metrics := promhttp.Handler()

	mux := goji.NewMux()
	mux.HandleFunc(pat.Get("/ws"), func(w http.ResponseWriter, r *http.Request) {
		server.Connect(w, r)
	})
	mux.HandleFunc(pat.Get("/metrics"), metrics.ServeHTTP)

	// Switch with zap.NewProduction() when needed
	// or even better, add a flag to switch this as needed.
	// Example: ./ptrun-server --debug
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	sugared := logger.Sugar()

	server = ws.NewServer(logger)
	go server.Run()

	// TODO: We should allow some overrides by passing parameters to our executable
	// Example: ./ptr-server --port 8080 --path "/ws"
	sugared.Infow("starting websocket endpoint",
		"addr", "0.0.0.0",
		"port", 8080,
		"path", "/ws")

	srv := &http.Server{
		Handler: mux,
		Addr:    "0.0.0.0:8080",
		// These should probably be moved under internal/const
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	metricsstate.Set(1)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			metricsstate.Set(2)
			sugared.Errorw("error starting http server", zap.Error(err))
		}
	}()

	<-done
	sugared.Debug("stopping")
	metricsstate.Set(-1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() {
		// If we need any more cleanup, it should go here

		// We should probably check and gracefully shutdown everything at this
		// point and also disconnect all our clients
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		sugared.Errorw("error shutting down http server", zap.Error(err))
	}
}
