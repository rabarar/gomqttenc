package main

import (
	"gomqttenc/shared"
	"gomqttenc/utils"

	"github.com/charmbracelet/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func makeHandler(ctx *shared.MqttMessageHandlerContext) mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {

		topic := msg.Topic()
		log.Infof("Received MQTT message from topic: \x1b[33m%s\x1b[0m", topic)

		for handlerName, handler := range ctx.Plugs {
			log.Infof("Topic Matches [%s] [%s]", handlerName, topic)
			if utils.TopicMatches(handlerName, topic) {
				err := handler.Process(topic, ctx, msg)
				if err != nil {
					log.Errorf("failed to process [%s] with handler [%s] error: [%s]", topic, handlerName, err)
				} else {
					log.Infof("Dispatched [%s] =>  [%s]", topic, handlerName)
				}
				return
			}
		}
	}
}
