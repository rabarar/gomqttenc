package parser

import (
	"fmt"
	"gomqttenc/shared"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

type TextMessageType string

const (
	DeepwoodBLEType   TextMessageType = "BLE"
	DeepwoodWIFIType  TextMessageType = "WIFI"
	DeepwoodProbeType TextMessageType = "Probe"

	ALERT_DETECTED = "DETECTED"
	ALERT_CLEARED  = "CLEARED"
)

type TextMessage struct {
	Time   int64
	Type   TextMessageType
	Parsed interface{}
}

type DeepwoodBLE struct {
	Envelope MessageEnvelope
	MACAddr  string
}

type DeepwoodWIFI struct {
	Envelope MessageEnvelope
	MACAddr  string
}

type DeepwoodProbe struct {
	Envelope MessageEnvelope
	MACAddr  string
}

func ParseTextMessage(msg string) (*TextMessage, error) {

	// Regular expression to extract 'type' and MAC address
	re := regexp.MustCompile(`Detected non-baseline (\w+): ((?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2})`)

	log.Debugf("Matching [%s]", msg)
	matches := re.FindStringSubmatch(msg)

	if matches == nil || len(matches) != 3 {
		return nil, fmt.Errorf("input string format invalid or no match found")
	}

	switch strings.ToLower(matches[1]) {
	case "ble":
		var tm = TextMessage{
			Time: time.Now().Unix(),
			Type: DeepwoodBLEType,
			Parsed: DeepwoodBLE{
				MACAddr: matches[2],
			},
		}
		return &tm, nil

	case "wifi":
		var tm = TextMessage{
			Time: time.Now().Unix(),
			Type: DeepwoodWIFIType,
			Parsed: DeepwoodWIFI{
				MACAddr: matches[2],
			},
		}
		return &tm, nil
	case "probereq":
		var tm = TextMessage{
			Time: time.Now().Unix(),
			Type: DeepwoodProbeType,
			Parsed: DeepwoodProbe{
				MACAddr: matches[2],
			},
		}
		return &tm, nil
	default:
		return nil, fmt.Errorf("invalid NON-Baseline Detection Type")
	}

}

func ProcessTextMessage(telegrafChannel chan shared.TelegrafChannelMessage, parsed interface{}, messageEnv MessageEnvelope) error {

	switch v := parsed.(type) {
	case DeepwoodBLE:
		log.Infof("processing Deepwood BLE for telegraf")
		telegrafChannel <- DeepwoodBLE{
			Envelope: messageEnv,
			MACAddr:  v.MACAddr,
		}
	case DeepwoodWIFI:
		log.Infof("processing Deepwood WIFI for telegraf")
		telegrafChannel <- DeepwoodWIFI{
			Envelope: messageEnv,
			MACAddr:  v.MACAddr,
		}
	case DeepwoodProbe:
		log.Infof("processing Deepwood Probe for telegraf")
		telegrafChannel <- DeepwoodProbe{
			Envelope: messageEnv,
			MACAddr:  v.MACAddr,
		}
	default:
		log.Errorf("%s", fmt.Sprintf("unknown TEXT_MESSAGE parse type: %T", v))
		return shared.ErrMeshHandlerError
	}

	return nil
}
