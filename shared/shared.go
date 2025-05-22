package shared

import mqtt "github.com/eclipse/paho.mqtt.golang"

type MqttPluginHandler interface {
	Process(name string, msg mqtt.Message) error
}
