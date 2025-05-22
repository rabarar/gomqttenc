package main

import (
	"encoding/json"
	"errors"
	"gomqttenc/rtl433"
	"gomqttenc/shared"
	"log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MshMqttHandler struct{}

func (m MshMqttHandler) Process(name string, data interface{}, msg mqtt.Message) error {

	var sd rtl433.RTL433SensorData

	telegrafChannel, ok := data.(chan shared.TelegrafChannelMessage)
	if !ok {
		return errors.New("failed to cast passed data as telegraf channel message type")
	}

	// Unmarshal the JSON into the struct
	if err := json.Unmarshal([]byte(msg.Payload()), &sd); err != nil {
		log.Fatalf("Error unmarshaling JSON: %v", err)
		return rtl433.ErrHandleRTL433Data
	}

	// publish to Telegraf
	telegrafChannel <- rtl433.RTL433SensorData{
		Time:         sd.Time,
		Model:        sd.Model,
		ID:           sd.ID,
		BatteryOK:    sd.BatteryOK,
		TemperatureC: sd.TemperatureC,
		Humidity:     sd.Humidity,
		Status:       sd.Status,
		MIC:          sd.MIC,
	}

	return nil
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
