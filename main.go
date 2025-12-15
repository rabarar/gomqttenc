package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"gomqttenc/shared"
	"gomqttenc/utils"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/charmbracelet/log"

	_ "github.com/rabarar/meshtool-go/public/radio"
)

var (
	channelKeys             = map[string]shared.Key{}
	channelKeysByChannelNum = map[uint32]shared.Key{}
	telegrafChannel         = make(chan shared.TelegrafChannelMessage)
)

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
	cfg, err := utils.LoadConfig("config.json")
	if err != nil {
		log.Error("Failed to load config: %s\n", err)
		return
	}

	// load cert
	pfx, err := os.ReadFile(cfg.TAKCerts.PKCS12)
	if err != nil {
		log.Error("Failed to load TAK Cert PKCS12 client file: %s\n", err)
		return
	}

	key, leaf, chain, err := pkcs12.DecodeChain(pfx, cfg.TAKCerts.Passwd)
	if err != nil {
		log.Error("Failed to decode TAK Cert PKCS12 client file: %s\n", err)
		return
	}

	clientCert := tls.Certificate{
		PrivateKey: key,
		Certificate: func() [][]byte {
			out := make([][]byte, 0, 1+len(chain))
			out = append(out, leaf.Raw) // leaf first
			for _, c := range chain {
				out = append(out, c.Raw) // then intermediates
			}
			return out
		}(),
		Leaf: leaf,
	}

	caCert, err := os.ReadFile(cfg.TAKCerts.CACert)
	if err != nil {
		log.Error("Failed to load ca file: %s\n", err)
		return
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		log.Error("Failed to append CA certs\n")
		return
	}

	tlsConfig := &tls.Config{
		ServerName: "tak-nl.iogentic.com",
		//RootCAs:      caPool,
		RootCAs:      nil,
		Certificates: []tls.Certificate{clientCert},
		//MinVersion:    tls.VersionTLS12,
		//Renegotiation: tls.RenegotiateNever,
	}

	log.Printf("\ntlsConfig ptr = %p\n", tlsConfig)
	tlsConfig.GetClientCertificate = func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		log.Printf("\t>>> server requested client cert; acceptable CAs: %d", len(cri.AcceptableCAs))
		if len(tlsConfig.Certificates) == 0 {
			log.Printf("\t>>> no client cert configured")
			return &tls.Certificate{}, nil
		}
		log.Printf("\t>>> returning client cert chain length: %d", len(tlsConfig.Certificates[0].Certificate))
		return &tlsConfig.Certificates[0], nil
	}

	if len(tlsConfig.Certificates) == 0 {
		log.Fatal("\t>>> no client certificate configured (tlsConfig.Certificates is empty)")
		return
	}
	log.Printf("client cert chain length: %d", len(tlsConfig.Certificates[0].Certificate))
	log.Printf("RootCAs set: %v", tlsConfig.RootCAs != nil)

	// Load Plugins
	var MqttPluginHandlers = make(shared.MqttPluginHandlers)

	for t, p := range cfg.Topics {
		handler, err := loadMqttPlugin(p.Name, p.Path)
		if err != nil {
			log.Fatalf("failed to load handler: Name: [%s] Path: [%s] Error: [%s]", p.Name, p.Path, err)
		}
		MqttPluginHandlers[t] = handler
		log.Infof("Plugin: [%s] for Topic: [%s]  => [%s]", p.Name, t, p.Path)
	}

	// setup telegraf publisher
	ctx, cancel := context.WithCancel(context.Background())

	// show keys for channels
	for _, entry := range cfg.B64Keys {
		for k, v := range entry {
			log.Infof("creating key %s, value %s", k, string(v))
			var key shared.Key
			key.Txt = string(v)
			key.Hex, err = base64.StdEncoding.DecodeString(string(v))
			if err != nil {
				log.Fatalf("Invalid base64 channel key: %s", err)
			}
			// check to see if it's a direct or channel key and add it accordingly

			// if it starts with a bang it's a node key (DIRECT)
			if len(k) > 0 && k[0] == '!' {
				channelKeys[k] = key
			} else {
				channelKeys[k] = key
				cHash := generateHash(k, key.Txt)
				channelKeysByChannelNum[cHash] = key
				log.Infof("creating hash key %s value %x", k, cHash)
			}
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

	takCerts := shared.TAKCerts{
		TLSClientConfig: tlsConfig,
	}

	opts.SetDefaultPublishHandler(makeHandler(&shared.MqttMessageHandlerContext{
		Plugs:                   MqttPluginHandlers,
		TelegrafChan:            telegrafChannel,
		ChannelKeys:             channelKeys,
		ChannelKeysByChannelNum: channelKeysByChannelNum,
		TAKServer:               cfg.TAKServer,
		TAKCerts:                takCerts,
	}))

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

	for topic, v := range cfg.Topics {
		log.Infof("subscribed to topic: ['%s'] with Qos: [%d]", topic, v.QoS)
	}

	// subscripe to MQTT Topic and process in messageHandler()
	if token := client.SubscribeMultiple(utils.TopicsQoSFromConfig(cfg.Topics), nil); token.Wait() && token.Error() != nil {
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
