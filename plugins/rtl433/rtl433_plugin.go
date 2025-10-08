package main

import (
	"encoding/json"
	"gomqttenc/rtl433"
	"gomqttenc/shared"

	"github.com/charmbracelet/log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MshMqttHandler struct{}

func (m MshMqttHandler) Process(name string, data interface{}, msg mqtt.Message) error {

	ctx, ok := data.(*shared.MqttMessageHandlerContext)
	if !ok {
		log.Error("failed to cast expected data to MqttMessageHandlerContext")
		return rtl433.ErrHandleRTL433Data
	}

	telegrafChannel, ok := (ctx.TelegrafChan).(chan shared.TelegrafChannelMessage)
	if !ok {
		log.Fatal("failed to cast expected data to chan shared.TelegrafChannelMessage")
	}

	var sd rtl433.RTL433SensorData

	// Unmarshal the JSON into the struct
	if err := json.Unmarshal([]byte(msg.Payload()), &sd); err != nil {
		log.Warnf("Error unmarshaling JSON: payload: [%s]  %v", msg.Payload(), err)
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
