//go:build !linux

package main

import "github.com/rustyeddy/devices/drivers"

func newGPIOFactory(_ bool) drivers.Factory {
	return drivers.NewVPIOFactory()
}

func newOLEDFactory(_ bool) drivers.OLEDFactory {
	return drivers.MockOLEDFactory{}
}
