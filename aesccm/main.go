package aesccm

import (
	"crypto/cipher"
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"fmt"
)

type CCM struct {
	block     cipher.Block
	nonceSize int
	tagSize   int
}

func NewCCM(block cipher.Block, tagSize int, nonceSize int) (*CCM, error) {
	if nonceSize < 7 || nonceSize > 13 {
		return nil, errors.New("ccm: invalid nonce size")
	}
	if tagSize < 4 || tagSize > 16 || tagSize%2 != 0 {
		return nil, errors.New("ccm: invalid tag size")
	}
	return &CCM{
		block:     block,
		nonceSize: nonceSize,
		tagSize:   tagSize,
	}, nil
}

// Open decrypts ciphertext and verifies MAC
func (c *CCM) Open(nonce, ciphertext, mac, aad []byte) ([]byte, error) {
	if len(nonce) != c.nonceSize {
		return nil, errors.New("ccm: invalid nonce length")
	}
	if len(mac) != c.tagSize {
		return nil, errors.New("ccm: invalid MAC length")
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("ccm: empty ciphertext")
	}

	// Setup AES-CTR stream for decryption
	L := 15 - c.nonceSize
	iv := make([]byte, 16)
	iv[0] = byte(L - 1)
	copy(iv[1:], nonce)

	ctr := cipher.NewCTR(c.block, iv)
	plaintext := make([]byte, len(ciphertext))
	ctr.XORKeyStream(plaintext, ciphertext)

	// Now compute expected MAC

	// Build B0 block
	b0 := make([]byte, 16)

	//NOTIONAL flags := byte((aad != nil && len(aad) > 0) << 6) // AAD flag
	var aadFlag byte
	if aad != nil && len(aad) > 0 {
		aadFlag = 1
	}

	flags := (aadFlag << 6) // Now this is lega
	flags |= byte(((c.tagSize - 2) / 2) << 3)
	flags |= byte(L - 1)
	b0[0] = flags
	copy(b0[1:1+c.nonceSize], nonce)
	binary := uint64(len(plaintext))
	for i := 0; i < L; i++ {
		b0[15-i] = byte(binary & 0xff)
		binary >>= 8
	}

	// Prepare MAC input
	macInput := make([]byte, 0, 64)
	macInput = append(macInput, b0...)
	if aad != nil && len(aad) > 0 {
		// Skipped AAD processing for now (most Meshtastic messages have no AAD)
	}
	macInput = append(macInput, plaintext...)

	// Calculate CBC-MAC using AES block cipher
	expectedMAC, err := c.calculateMAC(macInput)
	if err != nil {
		return nil, err
	}

	// Compare MACs
	if !hmac.Equal(mac, expectedMAC[:c.tagSize]) {
		return nil, errors.New(fmt.Sprintf("ccm: authentication failed (MAC mismatch): Expected: [%s] MAC: [%s]", hex.EncodeToString(expectedMAC[:c.tagSize]), hex.EncodeToString(mac)))
	}

	return plaintext, nil
}

// calculateMAC generates a CBC-MAC using AES block cipher
func (c *CCM) calculateMAC(input []byte) ([]byte, error) {
	if len(input)%16 != 0 {
		padLen := 16 - (len(input) % 16)
		pad := make([]byte, padLen)
		input = append(input, pad...)
	}

	mac := make([]byte, 16)
	tmp := make([]byte, 16)

	for i := 0; i < len(input); i += 16 {
		for j := 0; j < 16; j++ {
			tmp[j] = mac[j] ^ input[i+j]
		}
		c.block.Encrypt(mac, tmp)
	}

	return mac, nil
}
