package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rabarar/meshtastic"

	"github.com/charmbracelet/log"

	"github.com/rabarar/meshtool-go/public/radio"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"google.golang.org/protobuf/proto"
)

var (
	channelKeys = map[string][]byte{}
)

type Config struct {
	Broker   string              `json:"broker"`
	Topic    string              `json:"topic"`
	ClientID string              `json:"clientID"`
	Username string              `json:"username"`
	Password string              `json:"password"`
	B64Keys  []map[string]string `json:"b64Key"`
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func decryptMeshtasticAESGCM(channelKey []byte, senderID uint32, packetID uint32, ciphertextWithTag []byte) (string, error) {
	if len(channelKey) != 16 {
		return "", fmt.Errorf("channel key must be 16 bytes for AES-128")
	}

	block, err := aes.NewCipher(channelKey)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %v", err)
	}

	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint32(nonce[0:4], senderID)
	binary.LittleEndian.PutUint32(nonce[4:8], packetID)
	// last 4 bytes of nonce remain zero

	plaintext, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %v", err)
	}

	return string(plaintext), nil
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message from topic: %s", msg.Topic())

	var env meshtastic.ServiceEnvelope
	err := proto.Unmarshal(msg.Payload(), &env)
	if err != nil {
		log.Printf("Failed to parse MeshPacket: %v", err)
		return
	}

	if env.Packet == nil {
		log.Error("no packet in Service Envelop")
		return
	}

	// TODO check for key existence...
	messagePtr, err := radio.TryDecode(env.Packet, channelKeys[env.ChannelId])
	if err != nil {
		log.Error("failed to decode packet", "err", err, "payload", hex.EncodeToString(msg.Payload()))
		return
	}

	if out, err := processMessage(messagePtr); err != nil {
		if messagePtr.Portnum != 0 {
			log.Error("failed to process message", "err", err, "source", messagePtr.Source, "dest", messagePtr.Dest, "payload", hex.EncodeToString(msg.Payload()), "topic", msg.Topic(), "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
		}
		return
	} else {
		log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
	}

}

var ErrUnknownMessageType = errors.New("unknown message type")

func processMessage(message *meshtastic.Data) (string, error) {
	var err error
	if message.Portnum == meshtastic.PortNum_NODEINFO_APP {
		var user = meshtastic.User{}
		err = proto.Unmarshal(message.Payload, &user)
		return user.String(), err
	}
	if message.Portnum == meshtastic.PortNum_POSITION_APP {
		var pos = meshtastic.Position{}
		err = proto.Unmarshal(message.Payload, &pos)
		return pos.String(), err
	}
	if message.Portnum == meshtastic.PortNum_TELEMETRY_APP {
		var t = meshtastic.Telemetry{}
		err = proto.Unmarshal(message.Payload, &t)
		return t.String(), err
	}
	if message.Portnum == meshtastic.PortNum_NEIGHBORINFO_APP {
		var n = meshtastic.NeighborInfo{}
		err = proto.Unmarshal(message.Payload, &n)
		return n.String(), err
	}
	if message.Portnum == meshtastic.PortNum_STORE_FORWARD_APP {
		var s = meshtastic.StoreAndForward{}
		err = proto.Unmarshal(message.Payload, &s)
		return s.String(), err
	}
	if message.Portnum == meshtastic.PortNum_TEXT_MESSAGE_APP {
		txt := message.Payload
		return string(txt), err
	}
	if message.Portnum == meshtastic.PortNum_MAP_REPORT_APP {
		var m = meshtastic.MapReport{}
		err = proto.Unmarshal(message.Payload, &m)
		return m.String(), err
	}
	if message.Portnum == meshtastic.PortNum_TRACEROUTE_APP {
		txt := message.Payload
		return string(txt), err
	}
	if message.Portnum == meshtastic.PortNum_ROUTING_APP {
		var r = meshtastic.Routing{}
		err = proto.Unmarshal(message.Payload, &r)
		return r.String(), err
	}

	log.Printf("unknown messsage type: %d\n", message.Portnum)
	return "", ErrUnknownMessageType
}

func main() {

	var level string
	flag.StringVar(&level, "level", "info", "Log level")
	flag.Parse()

	if lvl, err := log.ParseLevel(level); err == nil {
		log.SetLevel(lvl)
	} else {
		log.Fatal("failed to parse log level", "level", level, "err", err)
	}

	cfg, err := loadConfig("config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	for _, entry := range cfg.B64Keys {
		for k, v := range entry {
			fmt.Printf("creating key for: %s value: %s\n", k, string(v))
			channelKeys[k], err = base64.StdEncoding.DecodeString(string(v))
			if err != nil {
				log.Fatal("Invalid base64 channel key:", err)
			}
		}
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetDefaultPublishHandler(messageHandler)

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("Error connecting to MQTT broker:", token.Error())
		os.Exit(1)
	}

	fmt.Println("Connected to MQTT broker")

	if token := client.Subscribe(cfg.Topic, 0, nil); token.Wait() && token.Error() != nil {
		fmt.Println("Subscription error:", token.Error())
		os.Exit(1)
	}

	fmt.Printf("Subscribed to topic '%s'\n", cfg.Topic)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Println("\nDisconnecting from MQTT broker")
	client.Disconnect(250)
	time.Sleep(time.Second)
}
