package pkg

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

var base32Alphabet = []byte("abcdefghijklmnopqrstuvwxyz012345")

// ShortURL generates a short code from input using the same algorithm as the
// legacy PHP implementation: MD5 the input, split into 4 groups of 8 hex chars,
// mask with 0x3FFFFFFF, extract 6 base-32 characters, return group[1].
func ShortURL(input string) string {
	hash := md5.Sum([]byte(input))
	hexStr := hex.EncodeToString(hash[:])

	// We have 32 hex chars, split into 4 groups of 8
	// Return group[1] (index 1)
	groups := make([]string, 4)
	for i := 0; i < 4; i++ {
		subHex := hexStr[i*8 : (i+1)*8]
		groups[i] = hexGroupToBase32(subHex)
	}

	return groups[1]
}

func hexGroupToBase32(subHex string) string {
	// Parse the 8 hex chars as a 32-bit integer
	var intVal uint32
	for _, c := range subHex {
		intVal = intVal<<4 | hexCharToUint32(byte(c))
	}

	// Mask with 0x3FFFFFFF (30 bits)
	intVal = intVal & 0x3FFFFFFF

	// Extract 6 base-32 characters
	result := make([]byte, 6)
	for j := 0; j < 6; j++ {
		val := intVal & 0x1F
		result[j] = base32Alphabet[val]
		intVal = intVal >> 5
	}

	return string(result)
}

func hexCharToUint32(c byte) uint32 {
	switch {
	case c >= '0' && c <= '9':
		return uint32(c - '0')
	case c >= 'a' && c <= 'f':
		return uint32(c - 'a' + 10)
	case c >= 'A' && c <= 'F':
		return uint32(c - 'A' + 10)
	default:
		return 0
	}
}

// GenerateRandomCode generates a random short code of the given length
// using the same base-32 alphabet (a-z0-5) with crypto/rand.
func GenerateRandomCode(length int) string {
	if length <= 0 {
		length = 6
	}
	result := make([]byte, length)
	max := big.NewInt(int64(len(base32Alphabet)))
	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback: should not happen with crypto/rand
			result[i] = base32Alphabet[0]
			continue
		}
		result[i] = base32Alphabet[idx.Int64()]
	}
	return string(result)
}
