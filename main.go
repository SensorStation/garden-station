package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rustyeddy/otto/logging"
	mqttclient "github.com/rustyeddy/otto/messenger/mqtt"
)

// Config holds runtime configuration for the gardener.
type Config struct {
	StationName  string
	Mock         bool
	DryThreshold float64
	WetThreshold float64
	Log          logging.Config
	Broker       string
	Username     string
}

var config Config

func init() {
	flag.BoolVar(&config.Mock, "mock", false, "mock GPIO and sensors (no hardware required)")
	flag.StringVar(&config.Broker, "mqtt-broker", "otto", "MQTT broker hostname")
	flag.StringVar(&config.Username, "mqtt-username", "", "MQTT username")
	flag.StringVar(&config.StationName, "station-name", "gardener", "station name")
	flag.Float64Var(&config.DryThreshold, "dry-threshold", 20.0, "soil moisture dry threshold (%)")
	flag.Float64Var(&config.WetThreshold, "wet-threshold", 60.0, "soil moisture wet threshold (%)")
	flag.StringVar(&config.Log.Level, "log-level", "info", "log level: debug, info, warn, error")
	flag.StringVar(&config.Log.Output, "log-output", "file", "log output: stdout, stderr, file")
	flag.StringVar(&config.Log.Format, "log-format", "text", "log format: text, json")
	flag.StringVar(&config.Log.FilePath, "log-file", "gardener.log", "log file path (when log-output=file)")
	config.Log.Output = "file"
	config.Log.Format = "text"
}

func main() {
	flag.Parse()

	if _, err := logging.NewService(config.Log); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	slog.Info("starting gardener",
		"station", config.StationName,
		"mock", config.Mock,
		"broker", config.Broker,
		"log_level", config.Log.Level,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// MQTT password from environment to avoid CLI process-list exposure.
	password := os.Getenv("MQTT_PASSWORD")

	var mqttPaho *mqttclient.Paho
	if !config.Mock && config.Broker != "" && config.Broker != "none" {
		mqttPaho = mqttclient.New(mqttclient.MQTTConfig{
			Broker:   "tcp://" + config.Broker + ":1883",
			Username: config.Username,
			Password: password,
		})
	}

	gardener := &Gardener{}
	if err := gardener.Init(ctx, mqttPaho); err != nil {
		log.Fatalf("gardener init failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := gardener.Run(ctx); err != nil {
			slog.Error("gardener stopped with error", "error", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
		<-done
	case <-done:
	}
	slog.Info("gardener stopped")
}
