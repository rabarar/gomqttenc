package main

import (
	"crypto/aes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	myccm "gomqttenc/aesccm"

	"github.com/charmbracelet/log"
	"github.com/rabarar/meshtastic"
	"golang.org/x/crypto/curve25519"
	"google.golang.org/protobuf/proto"
)

type DecryptType int
type KeyIndex uint8

const (
	DecryptChannel DecryptType = iota
	DecryptDirect
)
const (
	ReceiverKeyIndex KeyIndex = iota
	SenderKeyIndex
)

func TryDecode(packet *meshtastic.MeshPacket, keys []Key, decryptType DecryptType) (*meshtastic.Data, error) {

	switch packet.GetPayloadVariant().(type) {
	case *meshtastic.MeshPacket_Decoded:
		return packet.GetDecoded(), nil
	case *meshtastic.MeshPacket_Encrypted:
		var err error
		var decrypted []byte

		switch decryptType {
		case DecryptChannel:
			decrypted, err = XOR(packet.GetEncrypted(), keys[0].hex, packet.Id, packet.From)
			if err != nil {
				log.Warnf("Failed decrypting packet: %s", err)
				return nil, ErrDecrypt
			}
		case DecryptDirect:
			ciphertext := packet.GetEncrypted()
			log.Warnf("CIPHERTEXT: [%s][%d]", hex.EncodeToString(ciphertext), len(ciphertext))
			log.Warnf("PacketId: [%x]", packet.Id)

			decrypted, err = decryptCurve25519(keys, packet.From, packet.Id, packet.GetEncrypted())
			if err != nil {
				log.Warnf("Failed decrypting packet: %s", err)
				return nil, ErrDecrypt
			}

		}
		var meshPacket meshtastic.Data
		err = proto.Unmarshal(decrypted, &meshPacket)
		if err != nil {
			log.Warnf("Failed to unmarshal Meshtastic Data packet: %s", err)
			log.Warnf("Plaintext: [%x]", decrypted)
			return nil, ErrDecrypt
		}
		return &meshPacket, nil
	default:
		return nil, ErrUnkownPayloadType
	}
}

func sliceTo32ByteArray(slice []byte) (*[32]byte, error) {
	if len(slice) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d, [%x]", len(slice), slice)
	}
	var array [32]byte
	copy(array[:], slice)
	return &array, nil
}

func decryptCurve25519(privateKeys []Key, fromNode uint32, packetID uint32, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 12 {
		return nil, errors.New("ciphertext too short for auth")
	}

	// Step 1: Auth tag is last 12 bytes
	auth := ciphertext[len(ciphertext)-12:]
	encryptedData := ciphertext[:len(ciphertext)-12]

	// Step 2: Extract extraNonce (last 4 bytes of auth)
	extraNonce := binary.LittleEndian.Uint32(auth[8:12])

	// Step 3: Compute shared secret
	var receiverPriv [32]byte
	decodedPriv, _ := base64.StdEncoding.DecodeString(privateKeys[ReceiverKeyIndex].txt)
	copy(receiverPriv[:], decodedPriv)

	// Sender's private key
	// derive sender's public key
	keyslice, err := sliceTo32ByteArray(privateKeys[SenderKeyIndex].hex)
	if err != nil {
		log.Fatal(err)
	}
	senderPub, err := PublicKeyFromPrivateKey(*keyslice)
	if err != nil {
		log.Fatal(err)
	}

	// Compute shared secret
	sharedSecret, err := curve25519.X25519(receiverPriv[:], senderPub[:])
	if err != nil {
		log.Fatal(err)
	}

	// Step 4: Hash the shared secret with SHA-256
	hashedKey := sha256.Sum256(sharedSecret)

	// Step 5: Build 13-byte nonce
	nonce := buildNonce(packetID, fromNode, extraNonce)
	log.Warnf("nonce size is [%d]", len(nonce))

	// VERIFICATION STEP
	log.Warnf("Nonce (len %d): %x", len(nonce), nonce)
	log.Warnf("Shared key: %x", hashedKey[:])
	log.Warnf("Ciphertext (len %d): %x", len(encryptedData), encryptedData)
	log.Warnf("MAC: %x\n", auth[:8])
	log.Warnf("ExtraNonce (parsed): %x", auth[8:12])

	// Step 6: Decrypt AES-CCM using github.com/pschlump/AesCCM
	plaintext, err := decryptCCM(hashedKey[:], nonce, encryptedData, auth[:8])
	if err != nil {
		return nil, fmt.Errorf("AES-CCM decryption failed: %w", err)
	}

	return plaintext, nil
}
func decryptCCM(key, nonce, ciphertext, mac []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES block creation failed: %w", err)
	}

	ccm, err := myccm.NewCCM(block, 8, 13) // 8-byte MAC, 13-byte nonce
	if err != nil {
		return nil, fmt.Errorf("CCM setup failed: %w", err)
	}

	plaintext, err := ccm.Open(nonce, ciphertext, mac, nil)
	if err != nil {
		return nil, fmt.Errorf("CCM decryption failed: %w", err)
	}

	return plaintext, nil
}

func buildNonce(packetNum uint32, fromNode uint32, extraNonce uint32) []byte {
	nonce := make([]byte, 13)

	// packetNum low 32 bits (direct)
	binary.LittleEndian.PutUint32(nonce[0:4], packetNum)

	// full extraNonce
	binary.LittleEndian.PutUint32(nonce[4:8], extraNonce)

	// fromNode
	binary.LittleEndian.PutUint32(nonce[8:12], fromNode)

	// ⚡ DO NOT SET nonce[12]!
	// ⚡ It should remain zero (already zero by default in Go slices)

	return nonce
}

func PublicKeyFromPrivateKey(privKey [32]byte) ([32]byte, error) {
	pubKey, err := curve25519.X25519(privKey[:], curve25519.Basepoint)
	if err != nil {
		return [32]byte{}, err
	}
	var pubKeyFixed [32]byte
	copy(pubKeyFixed[:], pubKey)
	return pubKeyFixed, nil
}
