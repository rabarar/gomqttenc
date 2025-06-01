package main

import (
	"fmt"
	"gomqttenc/md"
	"gomqttenc/parser"
	"gomqttenc/shared"
	"gomqttenc/utils"

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
		log.Warnf("ignoring decoded payload: [%s]", mesh.GetDecoded())
	} else {

		fromKeyName := fmt.Sprintf("!%x", mesh.From)
		toKeyName := fmt.Sprintf("!%x", mesh.To)

		// compute Sender' public key from private key
		keyslice, err := utils.SliceTo32ByteArray(ctx.ChannelKeys[fromKeyName].Hex)
		if err != nil {
			log.Warnf("failed to SliceTo32Bytes: %s", err)
			return shared.ErrMeshHandlerError
		}
		senderPub, err := md.PublicKeyFromPrivateKey(*keyslice)
		if err != nil {
			log.Warnf("failed to extract public key from privte key: %s", err)
			return shared.ErrMeshHandlerError
		}

		decrypted, err := md.DecryptCurve25519(mesh.From, mesh.Id, senderPub[:], ctx.ChannelKeys[toKeyName].Hex, mesh.GetEncrypted())

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
		err = parser.ProcessTextMessage(telegrafChan, parsed.Parsed, messageEnv)
		if err != nil {
			log.Warnf("parse error: %s", err)
			return err
		}

	}
	return nil
}

// This symbol will be looked up
var Handler shared.MqttPluginHandler = MshMqttHandler{}
