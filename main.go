package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gomqttenc/github.com/meshtastic/go/generated"

	// Replace with actual path to your generated mesh.pb.go

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

	var packet generated.MeshPacket
	err := proto.Unmarshal(msg.Payload(), &packet)
	if err != nil {
		log.Printf("Failed to parse MeshPacket: %v", err)
		return
	}

	encryptedPayload, ok := packet.PayloadVariant.(*generated.MeshPacket_Encrypted)
	if !ok {
		log.Println("MeshPacket does not contain an Encrypted payload")
		return
	}

	decrypted, err := decryptMeshtasticAESGCM(channelKey, packet.From, packet.Id, encryptedPayload.Encrypted)
	if err != nil {
		log.Printf("Error decrypting payload: %v", err)
		return
	}
	fmt.Printf("Decrypted message: %s\n", decrypted)

}

func main() {

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
