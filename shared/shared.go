package shared

import mqtt "github.com/eclipse/paho.mqtt.golang"

type MqttPluginHandler interface {
	Process(name string, ctx interface{}, msg mqtt.Message) error
}

// PKI private Crypto key in both PEM and Hex Format
type Key struct {
	Hex []byte
	Txt string
}

// Generic Telegraf Channel Message to send to publisher
type TelegrafChannelMessage interface{}

// Application Config
type PluginConfig struct {
	Name string `json:"name"`
	Path string `json:"path"`
	QoS  byte   `json:"qos"`
}

// Config
type Config struct {
	Broker      string                  `json:"broker"`
	Topics      map[string]PluginConfig `json:"topics"`
	ClientID    string                  `json:"clientID"`
	Username    string                  `json:"username"`
	Password    string                  `json:"password"`
	B64Keys     []map[string]string     `json:"b64Key"`
	TelegrafURL string                  `json:"telegrafURL"`
}

// Plugins Map
type MqttPluginHandlers map[string]MqttPluginHandler

// MqttMessageHandlerContext

type MqttMessageHandlerContext struct {
	Plugs        MqttPluginHandlers
	TelegrafChan TelegrafChannelMessage
}
