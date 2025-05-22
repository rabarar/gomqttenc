package main

import (
	"gomqttenc/shared"

	"github.com/charmbracelet/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MshMqttHandler struct{}

func (m MshMqttHandler) Process(name string, data interface{}, msg mqtt.Message) error {
	log.Warnf("BLANK Processing for %s", name)
	return nil
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
