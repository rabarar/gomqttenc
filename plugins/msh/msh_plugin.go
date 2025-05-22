package main

import (
	"gomqttenc/shared"
	"strings"
)

type MshMqttHandler struct{}

func (m MshMqttHandler) Process(input string) string {
	return strings.ToUpper(input)
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
