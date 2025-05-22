package shared

type MqttPluginHandler interface {
	Process(input string) string
}
