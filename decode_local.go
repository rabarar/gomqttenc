package main

import (
	"fmt"

	"gomqttenc/md"
	"gomqttenc/shared"

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

func TryDecode(packet *meshtastic.MeshPacket, keys []shared.Key, decryptType DecryptType) (*meshtastic.Data, error) {

	switch packet.GetPayloadVariant().(type) {
	case *meshtastic.MeshPacket_Decoded:
		return packet.GetDecoded(), nil
	case *meshtastic.MeshPacket_Encrypted:
		var err error
		var decrypted []byte

		switch decryptType {
		case DecryptChannel:
			decrypted, err = XOR(packet.GetEncrypted(), keys[0].Hex, packet.Id, packet.From)
			if err != nil {
				log.Warnf("Failed decrypting packet: %s", err)
				return nil, ErrDecrypt
			}
		case DecryptDirect:
			/*
				ciphertext := packet.GetEncrypted()
				log.Warnf("CIPHERTEXT: [%s][%d]", hex.EncodeToString(ciphertext), len(ciphertext))
				log.Warnf("PacketId: [%x]", packet.Id)
				log.Warnf("From: [%x]", packet.From)
			*/

			// Sender's private key
			// derive sender's public key
			keyslice, err := sliceTo32ByteArray(keys[SenderKeyIndex].Hex)
			if err != nil {
				log.Fatal(err)
			}
			senderPub, err := PublicKeyFromPrivateKey(*keyslice)
			if err != nil {
				log.Fatal(err)
			}

			decrypted, err = md.DecryptCurve25519(packet.From, packet.Id, senderPub[:], keys[ReceiverKeyIndex].Hex, packet.GetEncrypted())

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

func PublicKeyFromPrivateKey(privKey [32]byte) ([32]byte, error) {
	pubKey, err := curve25519.X25519(privKey[:], curve25519.Basepoint)
	if err != nil {
		return [32]byte{}, err
	}
	var pubKeyFixed [32]byte
	copy(pubKeyFixed[:], pubKey)
	return pubKeyFixed, nil
}

func sliceTo32ByteArray(slice []byte) (*[32]byte, error) {
	if len(slice) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d, [%x]", len(slice), slice)
	}
	var array [32]byte
	copy(array[:], slice)
	return &array, nil
}
