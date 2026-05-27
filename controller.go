package main

import (
	"context"
	"log/slog"

	"github.com/rustyeddy/devices/devices/relay"
)

// Controller watches soil moisture and drives the pump automatically.
type Controller struct {
	dry    float64 // turn pump ON when moisture drops below this
	wet    float64 // turn pump OFF when moisture rises above this
	pumpOn bool
}

// handle evaluates moisture against thresholds and commands the pump.
func (c *Controller) handle(ctx context.Context, moisture float64, pump *relay.Relay) {
	switch {
	case moisture < c.dry && !c.pumpOn:
		slog.Info("soil dry; starting pump", "moisture", moisture, "threshold", c.dry)
		select {
		case pump.In() <- true:
			c.pumpOn = true
		case <-ctx.Done():
		}
	case moisture >= c.wet && c.pumpOn:
		slog.Info("soil wet; stopping pump", "moisture", moisture, "threshold", c.wet)
		select {
		case pump.In() <- false:
			c.pumpOn = false
		case <-ctx.Done():
		}
	}
}
