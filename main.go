package main

import (
	"bytes"
	"context"
	"encoding/base64"
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

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rabarar/meshtastic"

	"github.com/charmbracelet/log"

	_ "github.com/rabarar/meshtool-go/public/radio"

	"google.golang.org/protobuf/proto"
)

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
type Config struct {
	Broker      string              `json:"broker"`
	Topics      map[string]byte     `json:"topics"`
	ClientID    string              `json:"clientID"`
	Username    string              `json:"username"`
	Password    string              `json:"password"`
	B64Keys     []map[string]string `json:"b64Key"`
	TelegrafURL string              `json:"telegrafURL"`
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Warnf("failed to close config file")
		}
	}()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// MQTT callback for message handling
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Infof("Received MQTT message from topic: \x1b[33m%s\x1b[0m", msg.Topic())

	var err error
	switch getRootTopic(msg.Topic()) {
	case "msh":
		err = handleMeshtasticTopics(msg)
		if err != nil {
			log.Warnf("Failed to Handle Meshtastic Message: [%s]", err)
		}
	case "rtl_433":
		log.Infof("HANDLE RTL_433: Payload: [\x1b[33m%s\x1b[0m]", msg.Payload())
		err = handleRTL433Topics(msg)
		if err != nil {
			log.Warnf("Failed to Handle RTL_433 Message: [%s]", err)
		}
	default:
		log.Warnf("unknown topic: [%s]", msg.Topic())
	}

}

func handleMeshtasticTopics(msg mqtt.Message) error {
	var env meshtastic.ServiceEnvelope
	err := proto.Unmarshal(msg.Payload(), &env)
	if err != nil {
		log.Warnf("Failed to parse MeshPacket: Topic: [%s],  %v", msg.Topic(), err)
		return ErrMeshHandlerError
	}

	if env.Packet == nil {
		log.Error("no packet in Service Envelop")
		return ErrMeshHandlerError
	}

	log.Infof("SvsEnv|source: [%x] SvsEnv|dest: [%x]", env.Packet.From, env.Packet.To)
	log.Infof("SvsEnv|source pubKey: [%x]", env.Packet.GetPublicKey())

	// if it's a PKI message use the device ID to decrypt
	var privKeys []Key

	if env.ChannelId == "PKI" {

		encPacket := env.Packet.GetEncrypted()
		log.Warnf("ServicePacket Payload [%s]:[%d]", hex.EncodeToString(encPacket), len(encPacket))
		_, err := parseServiceEnvelopePayload(encPacket)
		if err != nil {
			log.Error("file to parse Service Envelop Payload")
			return ErrMeshHandlerError
		}

		// get both sender and receiver private keys
		toAddr := fmt.Sprintf("!%x", env.Packet.To)
		fromAddr := fmt.Sprintf("!%x", env.Packet.From)

		toAddrKey, ok := channelKeys[toAddr]
		if !ok {
			log.Errorf("PKI: no private key found for toAddr: [%s]", toAddr)
			return ErrMeshHandlerError
		}
		privKeys = append(privKeys, toAddrKey)
		log.Warnf("retrieving TO key for %s [%s]", toAddr, toAddrKey.txt)

		fromAddrKey, ok := channelKeys[fromAddr]
		if !ok {
			log.Errorf("PKI: no private key found for fromAddr: [%s]", fromAddr)
			return ErrMeshHandlerError
		}

		privKeys = append(privKeys, fromAddrKey)
		log.Warnf("retrieving FROM key for %s [%s]", fromAddr, fromAddrKey.txt)

	} else {
		log.Warnf("retrieving key for %s", env.ChannelId)
		privKey, ok := channelKeys[env.ChannelId]
		if !ok {
			log.Errorf("no private key found for ChannelId: [%s]", env.ChannelId)
			return ErrMeshHandlerError
		}
		log.Warnf("Decoding with key [%s]", privKey.txt)

		privKeys = append(privKeys, privKey)

	}

	var decryptType DecryptType
	switch getNthTopicSegmentFromEnd(msg.Topic(), 1) {
	case "PKI":
		decryptType = DecryptDirect
	default:
		decryptType = DecryptChannel
	}
	messagePtr, err := TryDecode(env.Packet, privKeys, decryptType)
	if err != nil {
		log.Error("failed to decode packet", "err", err, "payload", hex.EncodeToString(msg.Payload()))
		return ErrMeshHandlerError
	}

	if out, err := processMessage(messagePtr); err != nil {
		if messagePtr.Portnum != 0 {
			log.Error("failed to process message", "err", err, "source", messagePtr.Source, "dest", messagePtr.Dest, "payload", hex.EncodeToString(msg.Payload()), "topic", msg.Topic(), "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())
		}
		return ErrMeshHandlerError
	} else {
		log.Info(out, "topic", msg.Topic, "source", messagePtr.Source, "dest", messagePtr.Dest, "channel", env.ChannelId, "portnum", messagePtr.Portnum.String())

		log.Infof("parsing [%s]", out)
		messageEnv := MessageEnvelope{
			Device: env.Packet.From,
			From:   env.Packet.From,
			To:     env.Packet.To,
			Topic:  msg.Topic(),
		}

		switch messagePtr.Portnum {
		case meshtastic.PortNum_NODEINFO_APP:
			parsed, err := parseNodeInfoMessage(out)
			if err != nil {
				fmt.Println("Error parsing NODEINFO:", err)
				return ErrMeshHandlerError
			}
			log.Infof("Parsed NodeInfo Report Message:\n%+v\n", parsed)

			telegrafChannel <- NodeInfoMessage{
				Envelope:  messageEnv,
				Id:        parsed.Id,
				LongName:  parsed.LongName,
				ShortName: parsed.ShortName,
				MACaddr:   parsed.MACaddr,
				HWModel:   parsed.HWModel,
				PublicKey: parsed.PublicKey,
			}

		case meshtastic.PortNum_MAP_REPORT_APP:
			parsed, err := parseMapReportMessage(out)
			if err != nil {
				fmt.Println("Error parsing MAP_REPORT:", err)
				return ErrMeshHandlerError
			}
			log.Infof("Parsed Map Report Message:\n%+v\n", parsed)

			telegrafChannel <- MapReportMessage{
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

		case meshtastic.PortNum_POSITION_APP:
			parsed, err := parsePositionMessage(out)
			if err != nil {
				fmt.Println("Error:", err)
				return ErrMeshHandlerError
			}

			telegrafChannel <- PositionMessage{
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

		case meshtastic.PortNum_TELEMETRY_APP:
			parsed, err := parseTelemetryMessage(out)
			if err != nil {
				fmt.Println("Parse error:", err)
			} else {
				log.Infof("Parsed message: %+v\n", parsed)

				switch v := parsed.Parsed.(type) {
				case DeviceMetrics:

					telegrafChannel <- DeviceMetrics{
						Envelope:           messageEnv,
						BatteryLevel:       v.BatteryLevel,
						Voltage:            v.Voltage,
						ChannelUtilization: v.ChannelUtilization,
						AirUtilTx:          v.AirUtilTx,
						UptimeSeconds:      v.UptimeSeconds,
					}

				case EnvironmentMetrics:
					log.Infof("EnvironmentMetrics - Temp: %f Humidity: %f", v.Temperature, v.RelativeHumidity)

					telegrafChannel <- EnvironmentMetrics{
						Envelope:         messageEnv,
						Temperature:      v.Temperature,
						RelativeHumidity: v.RelativeHumidity,
					}

				default:
					fmt.Println("Unknown type")
					return ErrMeshHandlerError
				}
			}
		}
	}
	return nil
}

func handleRTL433Topics(msg mqtt.Message) error {

	var sd RTL433SensorData

	// Unmarshal the JSON into the struct
	if err := json.Unmarshal([]byte(msg.Payload()), &sd); err != nil {
		log.Fatalf("Error unmarshaling JSON: %v", err)
		return ErrHandleRTL433Data
	}

	// publish to Telegraf
	telegrafChannel <- RTL433SensorData{
		Time:         sd.Time,
		Model:        sd.Model,
		ID:           sd.ID,
		BatteryOK:    sd.BatteryOK,
		TemperatureC: sd.TemperatureC,
		Humidity:     sd.Humidity,
		Status:       sd.Status,
		MIC:          sd.MIC,
	}

	return nil
}

// Meshtastic message processing function unmarshaling and return the contents in a string
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

	log.Warn("Unknown messsage type: %d\n", message.Portnum)
	return "", ErrUnknownMessageType
}

// receive incoming telegraf Messages on the channel and publish to the Telegraf server
func startPublisher(ctx context.Context, wg *sync.WaitGroup, telegrafURL string, telegrafChannel chan TelegrafChannelMessage) {
	defer wg.Done()

	for {

		select {
		case msg := <-telegrafChannel:
			timestamp := time.Now().UnixNano()
			var line string

			switch metric := msg.(type) {
			case NodeInfoMessage:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=NODEINFO_APP "+
					"id=\"%s\",long_name=\"%s\",short_name=\"%s\",macaddr=\"%s\",hw_model=\"%s\",public_key=\"֡%s\"",
					metric.Envelope.Device, metric.Id, metric.LongName, metric.ShortName, metric.MACaddr, metric.HWModel, metric.PublicKey)

			case DeviceMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"battery_level=%d,voltage=%f,channel_utilization=%f,air_util_tx=%f,uptime_seconds=%d %d",
					metric.Envelope.Device, metric.BatteryLevel, metric.Voltage,
					metric.ChannelUtilization, metric.AirUtilTx, metric.UptimeSeconds, timestamp)

			case EnvironmentMetrics:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=TELEMETRY_APP "+
					"temperature=%f,relative_humidity=%f %d",
					metric.Envelope.Device, metric.Temperature, metric.RelativeHumidity, timestamp)

			case MapReportMessage:
				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=MAP_REPORT_APP "+
					"long_name=\"%s\",short_name=\"%s\",HwModel=\"%s\",FirmwareVersion=\"%s\",Region=\"%s\",HasDefaultChannel=%t,LatitudeI=%d,LongitudeI=%d,Altitude=%d,PositionPrecision=%d,NumOnlineLocalNodes=%d %d",
					metric.Envelope.Device, metric.LongName, metric.ShortName, metric.HwModel, metric.FirmwareVersion,
					metric.Region, metric.HasDefaultChannel, metric.LatitudeI, metric.LongitudeI, metric.Altitude,
					metric.PositionPrecision, metric.NumOnlineLocalNodes, timestamp)

			case PositionMessage:
				var ts int64
				var seq, sats int

				if metric.Timestamp == nil {
					ts = 0
				} else {
					ts = *metric.Timestamp
				}
				if metric.SeqNumber == nil {
					seq = 0
				} else {
					seq = *metric.SeqNumber
				}
				if metric.SatsInView == nil {
					sats = 0
				} else {
					sats = *metric.SatsInView
				}

				line = fmt.Sprintf("device_metrics,device=%x,channel=LongFast,portnum=POSITION_APP "+
					"LatitudeI=%d,LongitudeI=%d,Altitude=%d,Time=%d,LocationSource=\"%s\",Timestamp=%d,SeqNumber=%d,SatsInView=%d,GroundSpeed=%d,GroundTrack=%d,PrecisionBits=%d %d",
					metric.Envelope.Device,
					metric.LatitudeI, metric.LongitudeI, metric.Altitude, metric.Time, metric.LocationSource, ts, seq, sats, metric.GroundSpeed, metric.GroundTrack, metric.PrecisionBits, timestamp)

			case RTL433SensorData:
				line = fmt.Sprintf("rtl_433,model=\"%s\",ID=%d "+
					"TemperatureC=%f,Humidity=%d,BatteryOK=%d,Status=%d,MIC=\"%s\"",
					metric.Model, metric.ID, metric.TemperatureC, metric.Humidity, metric.BatteryOK, metric.Status, metric.MIC)
			default:
				log.Error("Unknown Telegraf Channel Message Type received -- no message published: %T", msg)
				return
			}

			// create and send request to the telegraf server
			req, err := http.NewRequest("POST", telegrafURL, bytes.NewBuffer([]byte(line)))
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
			err = resp.Body.Close()
			if err != nil {
				log.Error("Error failed to close Request Body:", err)
			}

			// TODO this isn't it!
			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
				log.Infof("metric published to Telegraf: %s", line)
			} else {
				log.Warnf("FAILED metric published to Telegraf StatusCode: %d, Status: %s", resp.StatusCode, resp.Status)
				log.Infof("metric published to Telegraf Line: [%s]", line)
			}

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
	go startPublisher(ctx, &wg, cfg.TelegrafURL, telegrafChannel)

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

	// check topics exist
	if len(cfg.Topics) == 0 {
		log.Fatal("Error no topics listed in json file, aborting")
		os.Exit(1)
	}

	for topic, QoS := range cfg.Topics {
		log.Infof("Subscribed to topic: ['%s'] with Qos: [%d]", topic, QoS)
	}

	// subscripe to MQTT Topic and process in messageHandler()
	if token := client.SubscribeMultiple(cfg.Topics, nil); token.Wait() && token.Error() != nil {
		log.Fatal("Subscription error:", token.Error())
		os.Exit(1)
	}

	// idle and wait for shutdowns
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
