package shared

import (
	"errors"

	"github.com/charmbracelet/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rabarar/meshtastic"
	"google.golang.org/protobuf/proto"
)

var (
	ErrUnknownMessageType = errors.New("unknown message type")
	ErrMeshHandlerError   = errors.New("failed to handle Mesh Topc")
)

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
	Plugs                   MqttPluginHandlers
	TelegrafChan            TelegrafChannelMessage
	ChannelKeys             map[string]Key
	ChannelKeysByChannelNum map[uint32]Key
}

// Meshtastic message processing function unmarshaling and return the contents in a string
func ProcessMessage(message *meshtastic.Data) (string, interface{}, error) {
	var err error
	if message.Portnum == meshtastic.PortNum_NODEINFO_APP {
		var user = meshtastic.User{}
		err = proto.Unmarshal(message.Payload, &user)
		return user.String(), &user, err
	}
	if message.Portnum == meshtastic.PortNum_POSITION_APP {
		var pos = meshtastic.Position{}
		err = proto.Unmarshal(message.Payload, &pos)
		return pos.String(), &pos, err
	}
	if message.Portnum == meshtastic.PortNum_TELEMETRY_APP {
		var t = meshtastic.Telemetry{}
		err = proto.Unmarshal(message.Payload, &t)
		return t.String(), &t, err
	}
	if message.Portnum == meshtastic.PortNum_NEIGHBORINFO_APP {
		var n = meshtastic.NeighborInfo{}
		err = proto.Unmarshal(message.Payload, &n)
		return n.String(), &n, err
	}
	if message.Portnum == meshtastic.PortNum_STORE_FORWARD_APP {
		var s = meshtastic.StoreAndForward{}
		err = proto.Unmarshal(message.Payload, &s)
		return s.String(), &s, err
	}
	if message.Portnum == meshtastic.PortNum_TEXT_MESSAGE_APP {
		txt := message.Payload
		return string(txt), nil, err
	}
	if message.Portnum == meshtastic.PortNum_MAP_REPORT_APP {
		var m = meshtastic.MapReport{}
		err = proto.Unmarshal(message.Payload, &m)
		return m.String(), &m, err
	}
	if message.Portnum == meshtastic.PortNum_TRACEROUTE_APP {
		var r = meshtastic.RouteDiscovery{}
		err = proto.Unmarshal(message.Payload, &r)
		return r.String(), &r, err
	}
	if message.Portnum == meshtastic.PortNum_ROUTING_APP {
		var r = meshtastic.Routing{}
		err = proto.Unmarshal(message.Payload, &r)
		return r.String(), &r, err
	}

	log.Warn("Unknown messsage type: %d\n", message.Portnum)
	return "", nil, ErrUnknownMessageType
}
