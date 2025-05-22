package utils

import "fmt"

func SliceTo32ByteArray(slice []byte) (*[32]byte, error) {
	if len(slice) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d, [%x]", len(slice), slice)
	}
	var array [32]byte
	copy(array[:], slice)
	return &array, nil
}
