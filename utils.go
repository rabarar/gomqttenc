package main

import (
	"errors"
	"strings"

	"github.com/charmbracelet/log"
)

type ParsedPacket struct {
	Ciphertext   []byte // Encrypted message
	MAC          []byte // 8-byte MAC
	ExtraNonce   byte   // Single byte (lowest byte from extraNonce)
	FullNonceRaw []byte // Full 4 bytes of extraNonce (debug)
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
	log.Warnf("Parsing ServiceEnvelope payload:")
	log.Warnf("- Ciphertext (%d bytes): %x", len(parsed.Ciphertext), parsed.Ciphertext)
	log.Warnf("- MAC (8 bytes): %x", parsed.MAC)
	log.Warnf("- ExtraNonce (4 bytes raw): %x (using byte: %02x)", parsed.FullNonceRaw, parsed.ExtraNonce)

	return parsed, nil
}
