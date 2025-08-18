package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/zeebo/blake3"
)

// hashFileToCID streams a file and generates a CID using BLAKE3
func HashFileToCID(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := blake3.New()

	// Stream the file into the hasher
	buf := make([]byte, 4*1024) // 4 KB buffer
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("error reading file: %w", err)
		}
		if n == 0 {
			break
		}
		hasher.Write(buf[:n]) // Write chunk to hasher
	}

	hash := hasher.Sum(nil)

	// Convert to multihash format
	mhash, err := mh.Encode(hash, mh.BLAKE3)
	if err != nil {
		return "", fmt.Errorf("failed to encode multihash: %w", err)
	}

	// Create CIDv1 with raw format
	c := cid.NewCidV1(cid.Raw, mhash)
	return c.String(), nil
}

// verifyFileIntegrity checks if a file's hash matches the original CID
func VerifyFileIntegrity(filePath, expectedCID string) (bool, error) {
	calculatedCID, err := HashFileToCID(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to hash file: %w", err)
	}
	return calculatedCID == expectedCID, nil
}

func RandomString(length int) string {
	bytes := make([]byte, length/2) // 2 hex chars per byte
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateCIDFromString creates a proper CID v1 (SHA-256) from a string key (used for DHT operations)
func CreateCIDFromString(key string) (cid.Cid, error) {
	hash, err := mh.Sum([]byte(key), mh.SHA2_256, -1)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to create hash: %w", err)
	}
	return cid.NewCidV1(cid.Raw, hash), nil
}
