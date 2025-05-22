package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/charmbracelet/log"

	_ "github.com/rabarar/meshtool-go/public/radio"
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
			log.Debug("creating key %s, value %s", k, string(v))
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

	log.Info("connected to MQTT broker")

	// check topics exist
	if len(cfg.Topics) == 0 {
		log.Fatal("Error no topics listed in json file, aborting")
		os.Exit(1)
	}

	for topic, QoS := range cfg.Topics {
		log.Infof("subscribed to topic: ['%s'] with Qos: [%d]", topic, QoS)
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
