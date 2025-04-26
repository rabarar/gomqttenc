package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rabarar/meshtastic"

	"github.com/charmbracelet/log"

	"github.com/rabarar/meshtool-go/public/radio"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"google.golang.org/protobuf/proto"
)

type Key struct {
	hex []byte
	txt string
}

var (
	ErrUnknownMessageType = errors.New("unknown message type")
	channelKeys           = map[string]Key{}
	lineCh                = make(chan Metric)
)

type Config struct {
	Broker   string              `json:"broker"`
	Topic    string              `json:"topic"`
	ClientID string              `json:"clientID"`
	Username string              `json:"username"`
	Password string              `json:"password"`
	B64Keys  []map[string]string `json:"b64Key"`
}

type Metric struct {
	Device             string
	BatteryLevel       int
	Voltage            float64
	ChannelUtilization float64
	AirUtilTx          float64
	UptimeSeconds      int
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

	log.Infof("SvsEnv|source: [%x] SvsEnv|dest: [%x]", env.Packet.From, env.Packet.To)

	// if it's a PKI message use the device ID to decrypt
	var privKey Key
	if env.ChannelId == "PKI" {
		env.ChannelId = fmt.Sprintf("!%x", env.Packet.To)
	}

	log.Warnf("retrieving key for %s", env.ChannelId)
	privKey, ok := channelKeys[env.ChannelId]
	if !ok {
		log.Errorf("no private key found for ChannelId: [%s]", env.ChannelId)
		return
	}

	log.Warnf("Decoding with key [%s]", privKey.txt)
	messagePtr, err := radio.TryDecode(env.Packet, privKey.hex)
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

		/* TODO
		lineCh <- Metric{
			Device:             device,
			BatteryLevel:       batteryLevel,
			Voltage:            voltage,
			ChannelUtilization: channelUtilization,
			AirUtilTx:          airUtilTx,
			UptimeSeconds:      uptimeSeconds,
		}
		*/

		log.Infof("parsing [%s]", out)
		switch messagePtr.Portnum {
		case meshtastic.PortNum_MAP_REPORT_APP:
			parsed, err := parseMapReportMessage(out)
			if err != nil {
				fmt.Println("Error parsing MAP_REPORT:", err)
				return
			}
			fmt.Printf("Parsed Map Report Message:\n%+v\n", parsed)
		case meshtastic.PortNum_POSITION_APP:
			parsed, err := parsePositionMessage(out)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			fmt.Printf("Parsed Position Message:\n%+v\n", parsed)
		case meshtastic.PortNum_TELEMETRY_APP:
			parsed, err := parseTelemetryMessage(out)
			if err != nil {
				fmt.Println("Parse error:", err)
			} else {
				fmt.Printf("Parsed message: %+v\n", parsed)

				switch v := parsed.Parsed.(type) {
				case DeviceMetrics:
					fmt.Println("DeviceMetrics - Battery:", v.BatteryLevel, "Voltage:", v.Voltage)
				case EnvironmentMetrics:
					fmt.Println("EnvironmentMetrics - Temp:", v.Temperature, "Humidity:", v.RelativeHumidity)
				default:
					fmt.Println("Unknown type")
				}
			}
		}
	}
}

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

func startPublisher(ctx context.Context, wg *sync.WaitGroup, lineCh <-chan Metric) {
	defer wg.Done()

	for {
		select {
		case metric := <-lineCh:
			timestamp := time.Now().UnixNano()

			line := fmt.Sprintf("device_metrics,device=%s,channel=LongFast,portnum=TELEMETRY_APP "+
				"battery_level=%d,voltage=%f,channel_utilization=%f,air_util_tx=%f,uptime_seconds=%d %d",
				metric.Device, metric.BatteryLevel, metric.Voltage,
				metric.ChannelUtilization, metric.AirUtilTx, metric.UptimeSeconds, timestamp)

			req, err := http.NewRequest("POST", "http://192.168.0.159:8186/telegraf", bytes.NewBuffer([]byte(line)))
			if err != nil {
				log.Error("Error creating request:", err)
				continue
			}
			req.Header.Set("Content-Type", "text/plain")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Error("Error posting to Telegraf:", err)
				continue
			}
			resp.Body.Close()

			log.Infof("Posted metric: %s", line)
			log.Infof("Response status: %s", resp.Status)

		case <-ctx.Done():
			log.Info("Publisher received shutdown signal (cancellled).")
			return
		}
	}
}

func main() {

	var level string
	flag.StringVar(&level, "level", "info", "Log level")
	flag.Parse()
	var wg sync.WaitGroup

	// setup logging
	if lvl, err := log.ParseLevel(level); err == nil {
		log.SetLevel(lvl)
	} else {
		log.Fatal("failed to parse log level", "level", level, "err", err)
	}

	// load config
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Error("Failed to load config: %v\n", err)
		return
	}

	// setup telegraf publisher
	ctx, cancel := context.WithCancel(context.Background())

	// show keys for channels
	for _, entry := range cfg.B64Keys {
		for k, v := range entry {
			log.Infof("creating key %s, value %s", k, string(v))
			var key Key
			key.txt = string(v)
			key.hex, err = base64.StdEncoding.DecodeString(string(v))
			if err != nil {
				log.Fatalf("Invalid base64 channel key: %s", err)
			}
			channelKeys[k] = key
		}
	}

	// Create signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		log.Info("Received interrupt. Cancelling...")
		cancel()
	}()
	wg.Add(1)

	// start telegraf publisher
	log.Info("starting telegraf publisher")
	go startPublisher(ctx, &wg, lineCh)

	// setup MQTT connection
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetDefaultPublishHandler(messageHandler)

	client := mqtt.NewClient(opts)

	// connect to MQTT broker
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal("Error connecting to MQTT broker:", token.Error())
		os.Exit(1)
	}

	log.Info("Connected to MQTT broker")

	// subscripe to MQTT Topic and process in messageHandler()
	if token := client.Subscribe(cfg.Topic, 0, nil); token.Wait() && token.Error() != nil {
		log.Fatal("Subscription error:", token.Error())
		os.Exit(1)
	}

	log.Infof("Subscribed to topic '%s'", cfg.Topic)

	// idle
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("Metric generator shutting down.")
				return
			default:
				time.Sleep(250 * time.Millisecond)
			}
		}
	}()

	wg.Wait()
	log.Info("All routines complete. Exiting.")
	// shutdown MQTT server
	log.Info("Disconnecting from MQTT broker")
	client.Disconnect(250)

	// terminate
	time.Sleep(time.Second)
	log.Info("shutdown complete, exitting")
}
