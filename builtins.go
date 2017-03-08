package main

type BuiltinImage struct {
	Name        string
	Description string
	Repository  string
	Commit      string
	Asset       string
}

var fwRegistry []BuiltinImage = []BuiltinImage{
	{"3c-qfw-2.0-30s", "Hamilton-3C, v2.0 30s interval", "https://github.com/hamilton-mote", "83fbc2e5f627723304dd432798ed8999bedc3332", "assets/3c-qfw-2.0-30s.bin"},
	{"3c-qfw-2.0-10s", "Hamilton-3C, v2.0 10s interval", "https://github.com/hamilton-mote", "83fbc2e5f627723304dd432798ed8999bedc3332", "assets/3c-qfw-2.0-10s.bin"},
}
