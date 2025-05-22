package utils

import (
	"encoding/json"
	"fmt"
	"gomqttenc/shared"
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

func TopicsQoSFromConfig(cfg map[string]shared.PluginConfig) map[string]byte {
	var transformed = make(map[string]byte)

	for topic, plug := range cfg {
		transformed[topic] = plug.QoS
	}

	return transformed
}

func LoadConfig(filename string) (*shared.Config, error) {
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

	var cfg shared.Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// topicMatches returns true if the received topic matches the subscription topic
func TopicMatches(subscription, received string) bool {
	// Case 1: No wildcard, exact match only
	if !strings.HasSuffix(subscription, "#") {
		return subscription == received
	}

	// Case 2: Wildcard '#'
	// Remove "/#" or just "#" from subscription
	if subscription == "#" {
		return true // Matches everything
	}

	if strings.HasSuffix(subscription, "/#") {
		prefix := strings.TrimSuffix(subscription, "/#")
		return received == prefix || strings.HasPrefix(received, prefix+"/")
	}

	// Unsupported wildcard usage (e.g., '#' not at end)
	return false
}

func GetRootTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func GetNthTopicSegmentFromEnd(topic string, n int) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}

	index := len(parts) - 1 - n
	if index < 0 || index >= len(parts) {
		return ""
	}

	return parts[index]
}

// replaceBinaryWithHex scans the string and replaces any non-printable characters
// (outside ASCII 32-126) with their hex-encoded form.
func ReplaceBinaryWithHex(input string) string {
	var b strings.Builder
	for _, r := range input {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("\\x%02X", r))
		}
	}
	return b.String()
}

// examine the payload and heuristically determine if it's a protobuf or a json packet
// nolint unused
func IsLikelyJSON(payload []byte) bool {
	// Trim leading whitespace and check if the first non-space char is '{'
	for _, b := range payload {
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		return (b == '{')
	}
	return false
}
