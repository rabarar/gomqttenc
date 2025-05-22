package main

import (
	"errors"
	"gomqttenc/shared"
	"plugin"

	"github.com/charmbracelet/log"
)

func loadMqttPlugin(name, path string) (shared.MqttPluginHandler, error) {

	p, err := plugin.Open(path)
	if err != nil {
		log.Errorf("failed to open plugin: %v", err)
		return nil, err
	}

	sym, err := p.Lookup("Handler")
	if err != nil {
		log.Errorf("failed to lookup Handler symbol: %v", err)
		return nil, err
	}

	handler, ok := sym.(*shared.MqttPluginHandler)
	if !ok {
		log.Errorf("unexpected type from module symbol")
		return nil, errors.New("unexpected type from module symbol")
	}

	return *handler, nil
}
