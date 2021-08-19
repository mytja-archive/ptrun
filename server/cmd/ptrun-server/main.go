package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"goji.io"
	"goji.io/pat"

	"github.com/ptr-geeks/ptrun/server/internal/ws"
)

var (
	server ws.Server
)

type ServerConfig struct {
	Debug bool
	Host  string
	Port  string
	Path  string
}

func main() {
	config := ServerConfig{}

	command := &cobra.Command{
		Use:   "ptrun-server",
		Short: "Game server for PTRun",
		Run: func(cmd *cobra.Command, args []string) {
			run(&config)
		},
	}

	command.Flags().BoolVar(&config.Debug, "debug", false, "enable debug mode")
	command.Flags().StringVar(&config.Host, "host", "0.0.0.0", "set server host")
	command.Flags().StringVar(&config.Port, "port", "8080", "set server port")
	command.Flags().StringVar(&config.Path, "path", "/ws", "set server WS path")

	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(config *ServerConfig) {
	var logger *zap.Logger
	var err error

	if config.Debug {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		panic(err)
	}

	sugared := logger.Sugar()

	server = ws.NewServer(logger)
	go server.Run()

	sugared.Infow("starting websocket endpoint",
		"host", config.Host,
		"port", config.Port,
		"path", config.Path)

	mux := goji.NewMux()
	mux.HandleFunc(pat.Get(config.Path), func(w http.ResponseWriter, r *http.Request) {
		server.Connect(w, r)
	})

	srv := &http.Server{
		Handler: mux,
		Addr:    fmt.Sprintf("%s:%s", config.Host, config.Port),
		// These should probably be moved under internal/const
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sugared.Errorw("error starting http server", zap.Error(err))
		}
	}()

	<-done
	sugared.Debug("stopping")

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
