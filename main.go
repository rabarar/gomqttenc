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
	channelKey = []byte{}
)

type Config struct {
	Broker   string `json:"broker"`
	Topic    string `json:"topic"`
	ClientID string `json:"clientID"`
	Username string `json:"username"`
	Password string `json:"password"`
	B64Key   string `json:"b64Key"`
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

	fmt.Println("Key length:", len(channelKey)) // should be 16 or 32

	var env meshtastic.ServiceEnvelope
	err := proto.Unmarshal(msg.Payload(), &env)
	if err != nil {
		log.Printf("Failed to parse MeshPacket: %v", err)
		return
	}

	log.Printf("ChannelID: [%s]\n", env.ChannelId)

	if env.Packet == nil {
		log.Error("no packet in Service Envelop")
		return
	}

	messagePtr, err := radio.TryDecode(env.Packet, channelKey)
	if err != nil {
		log.Error("failed to decode packet", "err", err, "payload", hex.EncodeToString(msg.Payload()))
		return
	}

	if out, err := processMessage(messagePtr); err != nil {
		if messagePtr.Portnum != 0 {
			log.Error("failed to process message", "err", err, "payload", hex.EncodeToString(msg.Payload()), "topic", msg.Topic(), "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
		}
		return
	} else {
		log.Info(out, "topic", msg.Topic, "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
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

	channelKey, err = base64.StdEncoding.DecodeString(cfg.B64Key)
	if err != nil {
		log.Fatal("Invalid base64 channel key:", err)
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
