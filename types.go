package main

import "errors"

// PKI private Crypto key in both PEM and Hex Format
type Key struct {
	hex []byte
	txt string
}

// Generic Telegraf Channel Message to send to publisher
type TelegrafChannelMessage interface{}

var (
	ErrUnknownMessageType = errors.New("unknown message type")
	ErrMeshHandlerError   = errors.New("failed to handle Mesh Topc")
	ErrHandleRTL433Data   = errors.New("failed to handle RTL433 Topc")
	channelKeys           = map[string]Key{}
	telegrafChannel       = make(chan TelegrafChannelMessage)
)

// Application Config
type PluginConfig struct {
	Name string `json:"name"`
	Path string `json:"path"`
	QoS  byte   `json:"qos"`
}

type Config struct {
	Broker      string                  `json:"broker"`
	Topics      map[string]PluginConfig `json:"topics"`
	ClientID    string                  `json:"clientID"`
	Username    string                  `json:"username"`
	Password    string                  `json:"password"`
	B64Keys     []map[string]string     `json:"b64Key"`
	TelegrafURL string                  `json:"telegrafURL"`
}
