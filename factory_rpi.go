//go:build linux && (arm || arm64)

package main

import "github.com/rustyeddy/devices/drivers"

func newGPIOFactory(mock bool) drivers.Factory {
	if mock {
		return drivers.NewVPIOFactory()
	}
	return drivers.NewGPIOCDevFactory()
}

func newOLEDFactory(mock bool) drivers.OLEDFactory {
	if mock {
		return drivers.MockOLEDFactory{}
	}
	return drivers.PeriphOLEDFactory{}
}
