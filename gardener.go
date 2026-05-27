package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rustyeddy/devices"
	bme280dev "github.com/rustyeddy/devices/devices/bme280"
	"github.com/rustyeddy/devices/devices/button"
	"github.com/rustyeddy/devices/devices/relay"
	"github.com/rustyeddy/devices/devices/vh400"
	"github.com/rustyeddy/devices/display"
	"github.com/rustyeddy/devices/drivers"
	"github.com/rustyeddy/devices/mock"
	"github.com/rustyeddy/otto/messenger"
	mqttclient "github.com/rustyeddy/otto/messenger/mqtt"
)

var pinmap = map[string]int{
	"on":   17,
	"off":  27,
	"pump": 5,
}

type Gardener struct {
	soil      devices.Source[float64]
	env       devices.Source[bme280dev.Env]
	pump      *relay.Relay
	buttonOn  *button.Button
	buttonOff *button.Button
	display   *display.OLED

	mqttClient *mqttclient.Paho
	registry   *messenger.Registry
	topics     messenger.TopicScheme

	ctrl *Controller
}

// Init creates all devices and wires MQTT subscriptions.
// Pass a non-nil mqttPaho to enable MQTT; pass nil to run in local-only mode.
func (g *Gardener) Init(ctx context.Context, mqttPaho *mqttclient.Paho) error {
	g.topics = messenger.TopicScheme{Prefix: "gardener"}
	g.ctrl = &Controller{
		dry: config.DryThreshold,
		wet: config.WetThreshold,
	}

	gpioFactory := newGPIOFactory(config.Mock)
	oledFactory := newOLEDFactory(config.Mock)

	if config.Mock {
		g.soil = mock.NewSensor(mock.SensorConfig[float64]{
			Name:        "soil",
			Interval:    10 * time.Second,
			Initial:     25.0,
			EmitInitial: true,
			Next: func(v float64) float64 {
				v -= 0.5
				if v < 10 {
					return 70.0
				}
				return v
			},
		})
		g.env = mock.NewSensor(mock.SensorConfig[bme280dev.Env]{
			Name:        "env",
			Interval:    10 * time.Second,
			Initial:     bme280dev.Env{Temperature: 22.0, Humidity: 55.0, Pressure: 101325.0},
			EmitInitial: true,
		})
	} else {
		g.soil = vh400.NewVH400(vh400.VH400Config{
			Name:        "soil",
			Factory:     drivers.PeriphADCFactory{},
			Bus:         "/dev/i2c-1",
			Addr:        0x48,
			Channel:     0,
			Interval:    10 * time.Second,
			EmitInitial: true,
		})
		g.env = bme280dev.New(bme280dev.Config{
			Name:        "env",
			Bus:         "/dev/i2c-1",
			Addr:        0x76,
			Interval:    10 * time.Second,
			EmitInitial: true,
		})
	}

	g.pump = relay.New(relay.RelayConfig{
		Name:    "pump",
		Factory: gpioFactory,
		Chip:    "gpiochip0",
		Offset:  pinmap["pump"],
	})
	g.buttonOn = button.NewButton(button.ButtonConfig{
		Name:    "on",
		Factory: gpioFactory,
		Chip:    "gpiochip0",
		Offset:  pinmap["on"],
		Edge:    drivers.EdgeBoth,
		Bias:    drivers.BiasPullUp,
	})
	g.buttonOff = button.NewButton(button.ButtonConfig{
		Name:    "off",
		Factory: gpioFactory,
		Chip:    "gpiochip0",
		Offset:  pinmap["off"],
		Edge:    drivers.EdgeBoth,
		Bias:    drivers.BiasPullUp,
	})
	g.display = display.NewOLED(display.OLEDConfig{
		Name:    "display",
		Factory: oledFactory,
		Bus:     "1",
		Addr:    0x27,
		Width:   128,
		Height:  64,
	})

	if mqttPaho != nil {
		g.mqttClient = mqttPaho
		g.registry = messenger.NewRegistry(mqttPaho, g.topics)

		// Subscribe to pump set commands from MQTT.
		pumpSetTopic := g.topics.Set("pump")
		g.registry.WantSub(pumpSetTopic, 1, func(m messenger.Message) {
			var on bool
			if err := json.Unmarshal(m.Payload, &on); err != nil {
				slog.Error("pump command parse failed", "payload", string(m.Payload), "error", err)
				return
			}
			select {
			case g.pump.In() <- on:
			default:
				slog.Warn("pump command dropped; buffer full")
			}
		})

		// ResubscribeAll is called automatically by SetOnConnect on every connect/reconnect.
		mqttPaho.SetOnConnect(func() {
			g.registry.ResubscribeAll(ctx)
		})

		if err := mqttPaho.Connect(ctx); err != nil {
			slog.Warn("MQTT connect failed; running without broker", "error", err)
			g.registry = nil
			g.mqttClient = nil
		}
	}

	return nil
}

// Run starts all device goroutines and blocks until ctx is cancelled.
func (g *Gardener) Run(ctx context.Context) error {
	g.InitApp()

	var wg sync.WaitGroup

	devList := []devices.Device{g.soil, g.env, g.pump, g.buttonOn, g.buttonOff, g.display}
	for _, d := range devList {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Run(ctx); err != nil {
				slog.Error("device stopped", "device", d.Name(), "error", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		g.drainSoil(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		g.drainEnv(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		g.watchButtons(ctx)
	}()

	wg.Wait()
	return nil
}

func (g *Gardener) drainSoil(ctx context.Context) {
	for {
		select {
		case v, ok := <-g.soil.Out():
			if !ok {
				return
			}
			slog.Info("soil moisture", "value", fmt.Sprintf("%.2f%%", v))
			g.publish(ctx, g.topics.State("soil"), v)
			g.updateDisplay(fmt.Sprintf("soil: %.1f%%", v))
			g.ctrl.handle(ctx, v, g.pump)
		case <-ctx.Done():
			return
		}
	}
}

func (g *Gardener) drainEnv(ctx context.Context) {
	for {
		select {
		case v, ok := <-g.env.Out():
			if !ok {
				return
			}
			slog.Info("env reading",
				"temperature", fmt.Sprintf("%.1f°C", v.Temperature),
				"humidity", fmt.Sprintf("%.1f%%", v.Humidity),
				"pressure", fmt.Sprintf("%.0fPa", v.Pressure),
			)
			g.publish(ctx, g.topics.State("env"), v)
		case <-ctx.Done():
			return
		}
	}
}

// watchButtons forwards physical button presses to the pump.
// With BiasPullUp: line LOW (false) means button pressed.
func (g *Gardener) watchButtons(ctx context.Context) {
	for {
		select {
		case state, ok := <-g.buttonOn.Out():
			if !ok {
				return
			}
			if !state {
				slog.Info("on button pressed; starting pump")
				select {
				case g.pump.In() <- true:
				default:
				}
			}
		case state, ok := <-g.buttonOff.Out():
			if !ok {
				return
			}
			if !state {
				slog.Info("off button pressed; stopping pump")
				select {
				case g.pump.In() <- false:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (g *Gardener) publish(ctx context.Context, topic string, v any) {
	if g.mqttClient == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("marshal failed", "topic", topic, "error", err)
		return
	}
	if err := g.mqttClient.Publish(ctx, topic, b, false, 0); err != nil {
		slog.Error("mqtt publish failed", "topic", topic, "error", err)
	}
}

func (g *Gardener) updateDisplay(text string) {
	if g.display == nil {
		return
	}
	// Best-effort; non-blocking.
	select {
	case g.display.In() <- display.OLEDCommand{Type: display.CmdClear}:
	default:
	}
	select {
	case g.display.In() <- display.OLEDCommand{Type: display.CmdText, X: 0, Y: 20, Text: text}:
	default:
	}
	select {
	case g.display.In() <- display.OLEDCommand{Type: display.CmdFlush}:
	default:
	}
}
