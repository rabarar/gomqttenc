package md

import (
	"crypto/aes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	psaesccm "github.com/pschlump/AesCCM"
	"golang.org/x/crypto/curve25519"
)

func buildNonce(packetID uint32, fromNode uint32, extraNonce uint32) []byte {
	nonce := make([]byte, 13)
	binary.LittleEndian.PutUint32(nonce[0:4], packetID)
	binary.LittleEndian.PutUint32(nonce[4:8], extraNonce)
	binary.LittleEndian.PutUint32(nonce[8:12], fromNode)
	return nonce
}

func DecryptCurve25519(
	fromNode uint32,
	packetID uint32,
	remotePubKey []byte,
	myPrivKey []byte,
	payload []byte,
) ([]byte, error) {
	if len(payload) < 12 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	ciphertext := payload[:len(payload)-12]
	auth := payload[len(payload)-12:]
	mac := auth[:8]
	extraNonceBytes := auth[8:]
	extraNonce := binary.LittleEndian.Uint32(extraNonceBytes)

	nonce := buildNonce(packetID, fromNode, extraNonce)

	// Derive shared secret
	sharedSecret, err := curve25519.X25519(myPrivKey, remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("failed deriving shared secret: %w", err)
	}

	hashedKey := sha256.Sum256(sharedSecret)

	// Setup AESCCM with pschlump/AesCCM
	block, err := aes.NewCipher(hashedKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create AES block: %w", err)
	}

	ccm, err := psaesccm.NewCCM(block, 8, 13) // tagSize=8, nonceSize=13
	if err != nil {
		return nil, fmt.Errorf("failed to create CCM context: %w", err)
	}

	// Decrypt
	fullCipher := append(ciphertext, mac...)
	plaintext, err := ccm.Open(nil, nonce, fullCipher, nil)
	if err != nil {
		return nil, fmt.Errorf("AES-CCM decryption failed: %w", err)
	}

	return plaintext, nil
}
