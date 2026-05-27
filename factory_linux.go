//go:build linux && !arm && !arm64

package main

import "github.com/rustyeddy/devices/drivers"

func newGPIOFactory(mock bool) drivers.Factory {
	if mock {
		return drivers.NewVPIOFactory()
	}
	return drivers.NewGPIOCDevFactory()
}

// OLED hardware drivers are only available on linux arm/arm64.
func newOLEDFactory(_ bool) drivers.OLEDFactory {
	return drivers.MockOLEDFactory{}
}
