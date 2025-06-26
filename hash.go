package main

import (
	"encoding/base64"
	"strings"
)

// generateHash combines a channel name and a key to produce a consistent XOR hash.
func generateHash(name, key string) uint32 {
	if key == "AQ==" {
		key = "1PG7OiApB1nwvP+rz05pAQ=="
	}
	// Base64 decode the key
	key = strings.ReplaceAll(strings.ReplaceAll(key, "-", "+"), "_", "/")
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return 0
	}

	hName := XorHash([]byte(name))
	hKey := XorHash(keyBytes)
	return uint32(hName ^ hKey)
}

// XorHash computes the XOR of all bytes in the input.
func XorHash(data []byte) int {
	result := 0
	for _, b := range data {
		result ^= int(b)
	}
	return result
}
