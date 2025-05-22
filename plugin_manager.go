package main

import (
	"gomqttenc/shared"
	"plugin"

	"github.com/charmbracelet/log"
)

var MqttPluginHandlers = map[string]shared.MqttPluginHandler{}

func loadMqttPlugin(name, path string) {
	p, err := plugin.Open(path)
	if err != nil {
		log.Fatalf("failed to open plugin: %v", err)
	}

	sym, err := p.Lookup("Handler")
	if err != nil {
		log.Fatalf("failed to lookup Handler symbol: %v", err)
	}

	handler, ok := sym.(*shared.MqttPluginHandler)
	if !ok {
		log.Fatalf("unexpected type from module symbol")
	}

	MqttPluginHandlers[name] = *handler
}
