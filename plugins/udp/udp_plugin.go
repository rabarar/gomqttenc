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

	return HandleUDPPacket(msg, telegrafChan, ctx.ChannelKeys, ctx.ChannelKeysByChannelNum)

}

func HandleUDPPacket(msg mqtt.Message, telegrafChannel chan shared.TelegrafChannelMessage, channelKeys map[string]shared.Key, channelKeysByChannelNum map[uint32]shared.Key) error {

	var mesh meshtastic.MeshPacket
	err := proto.Unmarshal(msg.Payload(), &mesh)
	if err != nil {
		log.Warnf("Failed to parse MeshPacket: Topic: [%s],  %v", msg.Topic(), err)
		return shared.ErrMeshHandlerError
	}

	messageEnv := parser.MessageEnvelope{
		Device: mesh.From,
		From:   mesh.From,
		To:     mesh.To,
		Topic:  msg.Topic(),
	}

	log.Warnf("From: [%x] To: [%x] Id: [%x] Channel: [%x], WantAck: [%v], ViaMqtt: [%v]",
		mesh.From, mesh.To, mesh.Id, mesh.Channel, mesh.WantAck, mesh.ViaMqtt)

	if !mesh.PkiEncrypted {
		if mesh.GetDecoded() != nil {
			log.Warnf("ignoring decoded payload: [%s]", mesh.GetDecoded())
		} else {
			var privKeys []shared.Key
			log.Warnf("encrypted payload: [%s]", hex.EncodeToString(mesh.GetEncrypted()))
			log.Debugf("retrieving key for %x", mesh.Channel)

			// TODO - need to create mapping for channel num to string
			privKey, ok := channelKeysByChannelNum[mesh.Channel]
			if !ok {
				log.Errorf("no private key found for Channel: [%x]", mesh.Channel)
				return shared.ErrMeshHandlerError
			}
			log.Debugf("Decoding with key [%s]", privKey.Txt)

			privKeys = append(privKeys, privKey)

			messagePtr, err := md.TryDecode(&mesh, privKeys, md.DecryptChannel)

			if err != nil {
				log.Error("failed to decode packet", "err", err, "payload", hex.EncodeToString(mesh.GetEncrypted()))
				return shared.ErrMeshHandlerError
			}

			if out, obj, err := shared.ProcessMessage(messagePtr); err != nil {
				if messagePtr.Portnum != 0 {
					log.Error("failed to process message", "err", err, "source", messagePtr.Source, "dest", messagePtr.Dest, "payload", hex.EncodeToString(msg.Payload()), "topic", msg.Topic(), "channel", mesh.Channel, "portnum", messagePtr.Portnum.String())
				}
				return shared.ErrMeshHandlerError
			} else {

				switch messagePtr.Portnum {

				case meshtastic.PortNum_TEXT_MESSAGE_APP:
					log.Infof("\x1b[7m")
					log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", mesh.Channel, "portnum", messagePtr.Portnum.String())
					log.Infof("\x1b[0m")

				case meshtastic.PortNum_POSITION_APP:
					pos, ok := obj.(*meshtastic.Position)
					if ok {
						log.Infof("\x1b[33;40")
						log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", mesh.Channel, "portnum", messagePtr.Portnum.String())
						log.Infof("\x1b[0m")

						resp, err := tak.PostTelemetryTAK(context.Background(),
							"https://192.168.1.154:18888", tak.Telemetry{
								SerialNumber: fmt.Sprintf("%8.8x", messageEnv.From),
								DateTime:     time.Now(),
								Latitude:     float64(*(pos.LatitudeI)) / 10_000_000.0,
								Longitude:    float64(*(pos.LongitudeI)) / 10_000_000.0,
								Event:        "event",
								SolarPower:   "solar",
								Speed:        "Speed",
								Heading:      0,
							}, true)
						if err != nil {
							log.Errorf("failed to post to TAK Server: %s", err)
							return err
						}
						log.Infof("POSITION: POST to TAK Server: %s", resp)
					}

				default:
					log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", mesh.Channel, "portnum", messagePtr.Portnum.String())
				}

				log.Debugf("parsing [%s]", out)
				messageEnv := parser.MessageEnvelope{
					Device: mesh.From,
					From:   mesh.From,
					To:     mesh.To,
					Topic:  msg.Topic(),
				}
				log.Debugf("message Env: [%v]", messageEnv)
				// TODO need to add telegraf publishing  (from msh - create shared code..)
			}

		}
	} else {

		fromKeyName := fmt.Sprintf("!%x", mesh.From)
		toKeyName := fmt.Sprintf("!%x", mesh.To)

		// compute Sender' public key from private key
		keyslice, err := utils.SliceTo32ByteArray(channelKeys[fromKeyName].Hex)
		if err != nil {
			log.Warnf("failed to SliceTo32Bytes: %s", err)
			return shared.ErrMeshHandlerError
		}
		senderPub, err := md.PublicKeyFromPrivateKey(*keyslice)
		if err != nil {
			log.Warnf("failed to extract public key from privte key: %s", err)
			return shared.ErrMeshHandlerError
		}

		decrypted, err := md.DecryptCurve25519(mesh.From, mesh.Id, senderPub[:], channelKeys[toKeyName].Hex, mesh.GetEncrypted())

		if err != nil {
			log.Warnf("failed to decrypting packet: %s", err)
			return shared.ErrMeshHandlerError
		}
		plaintext := utils.TrimAll(string(decrypted))
		log.Infof("decrypted: [%s]", plaintext)

		parsed, err := parser.ParseTextMessage(plaintext)
		if err != nil {
			log.Warnf("failed to parse text message: %s", err)
			return shared.ErrMeshHandlerError
		}

		// TODO PROCESS TEXT MESSAGE
		err = parser.ProcessTextMessage(telegrafChannel, parsed.Parsed, messageEnv)
		if err != nil {
			log.Warnf("parse error: %s", err)
			return err
		}

	}
	return nil
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
