package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"gomqttenc/md"
	"gomqttenc/parser"
	"gomqttenc/shared"
	"gomqttenc/tak"
	"gomqttenc/utils"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rabarar/meshtastic"
	"google.golang.org/protobuf/proto"
)

type MshMqttHandler struct{}

func (m MshMqttHandler) Process(name string, data interface{}, msg mqtt.Message) error {

	ctx, ok := data.(*shared.MqttMessageHandlerContext)
	if !ok {
		log.Fatal("failed to cast expected data to MqttMessageHandlerContext")
	}

	telegrafChan, ok := (ctx.TelegrafChan).(chan shared.TelegrafChannelMessage)
	if !ok {
		log.Fatal("failed to cast expected data to chan shared.TelegrafChannelMessage")
	}

	return handleMeshtasticTopics(msg, telegrafChan, ctx.ChannelKeys, ctx.TAKServer)

}

func handleMeshtasticTopics(msg mqtt.Message, telegrafChannel chan shared.TelegrafChannelMessage, channelKeys map[string]shared.Key, TAKServer string) error {

	// TODO DEBUG JSON guessing ...

	if utils.IsLikelyJSON(msg.Payload()) {
		log.Warnf("msg is likely JSON: [%s]", msg.Payload())
		return nil
	}

	var env meshtastic.ServiceEnvelope
	err := proto.Unmarshal(msg.Payload(), &env)
	if err != nil {
		log.Warnf("Failed to parse MeshPacket: Topic: [%s],  %v", msg.Topic(), err)
		return shared.ErrMeshHandlerError
	}

	if env.Packet == nil {
		log.Error("nil packet in Service Envelop")
		log.Warnf("full envelop [%+v]", hex.EncodeToString(msg.Payload()))
		return shared.ErrMeshHandlerError
	}

	log.Infof("SvsEnv|source: [%x] SvsEnv|dest: [%x]", env.Packet.From, env.Packet.To)
	if len(env.Packet.GetPublicKey()) > 0 {
		log.Infof("SvsEnv|source pubKey: [%x]", env.Packet.GetPublicKey())
	}

	// if it's a PKI message use the device ID to decrypt
	var privKeys []shared.Key

	if env.ChannelId == "PKI" {

		encPacket := env.Packet.GetEncrypted()
		log.Infof("ServicePacket Payload [%s]:[%d]", hex.EncodeToString(encPacket), len(encPacket))
		_, err := md.ParseServiceEnvelopePayload(encPacket)
		if err != nil {
			log.Error("file to parse Service Envelop Payload")
			return shared.ErrMeshHandlerError
		}

		// get both sender and receiver private keys
		toAddr := fmt.Sprintf("!%x", env.Packet.To)
		fromAddr := fmt.Sprintf("!%x", env.Packet.From)

		toAddrKey, ok := channelKeys[toAddr]
		if !ok {
			log.Errorf("PKI: no private key found for toAddr: [%s]", toAddr)
			return shared.ErrMeshHandlerError
		}
		privKeys = append(privKeys, toAddrKey)
		log.Debugf("retrieving TO key for %s [%s]", toAddr, toAddrKey.Txt)

		fromAddrKey, ok := channelKeys[fromAddr]
		if !ok {
			log.Errorf("PKI: no private key found for fromAddr: [%s]", fromAddr)
			return shared.ErrMeshHandlerError
		}

		privKeys = append(privKeys, fromAddrKey)
		log.Debugf("retrieving FROM key for %s [%s]", fromAddr, fromAddrKey.Txt)

	} else {
		log.Debugf("retrieving key for %s", env.ChannelId)
		privKey, ok := channelKeys[env.ChannelId]
		if !ok {
			log.Errorf("no private key found for ChannelId: [%s]", env.ChannelId)
			return shared.ErrMeshHandlerError
		}
		log.Debugf("Decoding with key [%s]", privKey.Txt)

		privKeys = append(privKeys, privKey)

	}

	var decryptType md.DecryptType
	switch utils.GetNthTopicSegmentFromEnd(msg.Topic(), 1) {
	case "PKI":
		decryptType = md.DecryptDirect
	default:
		decryptType = md.DecryptChannel
	}
	messagePtr, err := md.TryDecode(env.Packet, privKeys, decryptType)
	if err != nil {
		log.Error("failed to decode packet", "err", err, "payload", hex.EncodeToString(msg.Payload()))
		return shared.ErrMeshHandlerError
	}

	if out, _, err := shared.ProcessMessage(messagePtr); err != nil {
		if messagePtr.Portnum != 0 {
			log.Error("failed to process message", "err", err, "source", messagePtr.Source, "dest", messagePtr.Dest, "payload", hex.EncodeToString(msg.Payload()), "topic", msg.Topic(), "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
		}
		return shared.ErrMeshHandlerError
	} else {
		log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())

		log.Debugf("parsing [%s]", out)
		messageEnv := parser.MessageEnvelope{
			Device: env.Packet.From,
			From:   env.Packet.From,
			To:     env.Packet.To,
			Topic:  msg.Topic(),
		}

		switch messagePtr.Portnum {
		case meshtastic.PortNum_NODEINFO_APP:
			parsed, err := parser.ParseNodeInfoMessage(out)
			if err != nil {
				fmt.Println("Error parsing NODEINFO:", err)
				return shared.ErrMeshHandlerError
			}
			log.Debugf("Parsed NodeInfo Report Message:\n%+v\n", parsed)

			telegrafChannel <- parser.NodeInfoMessage{
				Envelope:  messageEnv,
				Id:        parsed.Id,
				LongName:  parsed.LongName,
				ShortName: parsed.ShortName,
				MACaddr:   parsed.MACaddr,
				HWModel:   parsed.HWModel,
				PublicKey: parsed.PublicKey,
			}

		case meshtastic.PortNum_MAP_REPORT_APP:
			parsed, err := parser.ParseMapReportMessage(out)
			if err != nil {
				fmt.Println("Error parsing MAP_REPORT:", err)
				return shared.ErrMeshHandlerError
			}
			log.Infof("Parsed Map Report Message:\n%+v\n", parsed)

			telegrafChannel <- parser.MapReportMessage{
				Envelope:            messageEnv,
				LongName:            parsed.LongName,
				ShortName:           parsed.ShortName,
				HwModel:             parsed.HwModel,
				FirmwareVersion:     parsed.FirmwareVersion,
				Region:              parsed.Region,
				HasDefaultChannel:   parsed.HasDefaultChannel,
				LatitudeI:           parsed.LatitudeI,
				LongitudeI:          parsed.LongitudeI,
				Altitude:            parsed.Altitude,
				PositionPrecision:   parsed.PositionPrecision,
				NumOnlineLocalNodes: parsed.NumOnlineLocalNodes,
			}

			serial, err := strconv.Atoi(parsed.ShortName)
			if err != nil {
				fmt.Printf("Error converting %s to int: %s", parsed.ShortName, err)
				return shared.ErrMeshHandlerError
			}

			respBody, err := tak.PostTelemetryTAK(context.Background(),
				TAKServer, tak.Telemetry{
					SerialNumber: float64(serial),
					DateTime:     time.Now(),
					Latitude:     float64(parsed.LatitudeI) / 10_000_000.0,
					Longitude:    float64(parsed.LongitudeI) / 10_000_000.0,
					Event:        0,
					SolarPower:   0.0,
					Speed:        0.0,
					Heading:      0,
				}, true)
			if err != nil {
				log.Errorf("failed to post to TAK Server: %s", err)
				return err
			}
			log.Infof("MAP: POST to TAK Server: %s", respBody)

		case meshtastic.PortNum_POSITION_APP:
			parsed, err := parser.ParsePositionMessage(out)
			if err != nil {
				fmt.Println("Error:", err)
				return shared.ErrMeshHandlerError
			}

			telegrafChannel <- parser.PositionMessage{
				Envelope:       messageEnv,
				LatitudeI:      parsed.LatitudeI,
				LongitudeI:     parsed.LongitudeI,
				Altitude:       parsed.Altitude,
				Time:           parsed.Time,
				LocationSource: parsed.LocationSource,
				Timestamp:      parsed.Timestamp,
				SeqNumber:      parsed.SeqNumber,
				SatsInView:     parsed.SatsInView,
				GroundSpeed:    parsed.GroundSpeed,
				GroundTrack:    parsed.GroundTrack,
				PrecisionBits:  parsed.PrecisionBits,
			}

			respBody, err := tak.PostTelemetryTAK(context.Background(),
				TAKServer, tak.Telemetry{
					SerialNumber: float64(messageEnv.From),
					DateTime:     time.Now(),
					Latitude:     float64(parsed.LatitudeI),
					Longitude:    float64(parsed.LongitudeI),
					Event:        0,
					SolarPower:   0.0,
					Speed:        0.0,
					Heading:      0,
				}, true)
			if err != nil {
				log.Errorf("failed to post to TAK Server: %s", err)
				return err
			}
			log.Infof("POSITION: POST to TAK Server: %s", respBody)

		case meshtastic.PortNum_TEXT_MESSAGE_APP:

			parsed, err := parser.ParseTextMessage(out)
			if err != nil {
				log.Errorf("parse error: %s", err)
				return err
			}
			err = parser.ProcessTextMessage(telegrafChannel, parsed.Parsed, messageEnv)
			if err != nil {
				log.Warnf("parse error: %s", err)
				return err
			}

		case meshtastic.PortNum_TELEMETRY_APP:
			parsed, err := parser.ParseTelemetryMessage(out)
			if err != nil {
				log.Warnf("parse error: %s", err)
				return err
			} else {
				log.Infof("Parsed message: %+v", parsed)

				switch v := parsed.Parsed.(type) {
				case parser.DeviceMetrics:

					telegrafChannel <- parser.DeviceMetrics{
						Envelope:           messageEnv,
						BatteryLevel:       v.BatteryLevel,
						Voltage:            v.Voltage,
						ChannelUtilization: v.ChannelUtilization,
						AirUtilTx:          v.AirUtilTx,
						UptimeSeconds:      v.UptimeSeconds,
					}

				case parser.EnvironmentMetrics:
					log.Infof("EnvironmentMetrics - Temp: %f Humidity: %f", v.Temperature, v.RelativeHumidity)

					telegrafChannel <- parser.EnvironmentMetrics{
						Envelope:         messageEnv,
						Temperature:      v.Temperature,
						RelativeHumidity: v.RelativeHumidity,
					}

				default:
					fmt.Println("Unknown type")
					return shared.ErrMeshHandlerError
				}
			}
		}
	}
	return nil
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
