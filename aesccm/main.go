package aesccm

import (
	"crypto/cipher"
	"errors"
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

func (c *CCM) Open(nonce, ciphertext, mac, aad []byte) ([]byte, error) {
	if len(nonce) != c.nonceSize {
		return nil, errors.New("ccm: invalid nonce length")
	}
	if len(mac) != c.tagSize {
		return nil, errors.New("ccm: invalid mac length")
	}

	if len(ciphertext) == 0 {
		return nil, errors.New("ccm: no ciphertext")
	}

	L := 15 - c.nonceSize

	// Build the full counter IV for CTR mode
	iv := make([]byte, 16)
	iv[0] = byte(L - 1)
	copy(iv[1:], nonce)

	ctr := cipher.NewCTR(c.block, iv)

	plaintext := make([]byte, len(ciphertext))
	ctr.XORKeyStream(plaintext, ciphertext)

	// Verify the MAC
	expectedMAC, err := c.calculateMAC(nonce, aad, plaintext)
	if err != nil {
		return nil, err
	}
	for i := 0; i < c.tagSize; i++ {
		if expectedMAC[i] != mac[i] {
			return nil, errors.New("ccm: authentication failed")
		}
	}

	return plaintext, nil
}

func (c *CCM) calculateMAC(nonce, aad, plaintext []byte) ([]byte, error) {
	b0 := make([]byte, 16)
	flags := byte(0)
	if len(aad) > 0 {
		flags |= 0x40
	}
	flags |= byte(((c.tagSize - 2) / 2) << 3)
	flags |= byte(15 - c.nonceSize)
	b0[0] = flags
	copy(b0[1:1+c.nonceSize], nonce)

	payloadLen := uint64(len(plaintext))
	for i := uint(0); i < uint(15-c.nonceSize); i++ {
		b0[15-i] = byte(payloadLen & 0xff)
		payloadLen >>= 8
	}

	cmac := make([]byte, 16)
	c.block.Encrypt(cmac, b0)

	// Add AAD
	if len(aad) > 0 {
		aadLenBlock := make([]byte, 2)
		aadLenBlock[0] = byte(len(aad) >> 8)
		aadLenBlock[1] = byte(len(aad))
		cmac = xorBlock(cmac, aadLenBlock)
		copyBlock := make([]byte, 16)
		copy(copyBlock, aad)
		for len(copyBlock) < 16 {
			copyBlock = append(copyBlock, 0x00)
		}
		cmac = xorBlock(cmac, copyBlock)
		c.block.Encrypt(cmac, cmac)
	}

	// Add plaintext
	for i := 0; i < len(plaintext); i += 16 {
		block := make([]byte, 16)
		n := copy(block, plaintext[i:])
		if n < 16 {
			for j := n; j < 16; j++ {
				block[j] = 0
			}
		}
		cmac = xorBlock(cmac, block)
		c.block.Encrypt(cmac, cmac)
	}

	return cmac[:c.tagSize], nil
}

func xorBlock(a, b []byte) []byte {
	result := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] ^ b[i]
	}
	return result
}
