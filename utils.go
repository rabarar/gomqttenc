package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

type ParsedPacket struct {
	Ciphertext   []byte // Encrypted message
	MAC          []byte // 8-byte MAC
	ExtraNonce   byte   // Single byte (lowest byte from extraNonce)
	FullNonceRaw []byte // Full 4 bytes of extraNonce (debug)
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

func getRootTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func getNthTopicSegmentFromEnd(topic string, n int) string {
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

// Given full encrypted payload, parse ciphertext, MAC, and extraNonce
func parseServiceEnvelopePayload(payload []byte) (*ParsedPacket, error) {
	if len(payload) < 12 {
		return nil, errors.New("payload too short")
	}

	auth := payload[len(payload)-12:]       // last 12 bytes
	ciphertext := payload[:len(payload)-12] // everything before auth
	mac := auth[:8]                         // first 8 bytes
	extraNonceBytes := auth[8:12]           // last 4 bytes

	parsed := &ParsedPacket{
		Ciphertext:   ciphertext,
		MAC:          mac,
		ExtraNonce:   extraNonceBytes[0], // ONLY the low byte used
		FullNonceRaw: extraNonceBytes,
	}

	// Debug print
	/*
		log.Warnf("Parsing ServiceEnvelope payload:")
		log.Warnf("- Ciphertext (%d bytes): %x", len(parsed.Ciphertext), parsed.Ciphertext)
		log.Warnf("- MAC (8 bytes): %x", parsed.MAC)
		log.Warnf("- ExtraNonce (4 bytes raw): %x (using byte: %02x)", parsed.FullNonceRaw, parsed.ExtraNonce)
	*/

	return parsed, nil
}

// replaceBinaryWithHex scans the string and replaces any non-printable characters
// (outside ASCII 32-126) with their hex-encoded form.
func replaceBinaryWithHex(input string) string {
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
func isLikelyJSON(payload []byte) bool {
	// Trim leading whitespace and check if the first non-space char is '{'
	for _, b := range payload {
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		return (b == '{')
	}
	return false
}
