// Package connectedcompat recognizes one code-owned connected state contract.
// Recognition is not publisher authenticity, transition or activation authority.
// It imports no collector, filesystem, transport or installation implementation.
package connectedcompat

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
)

//go:embed descriptor.json
var descriptor string

var compiledDigest = func() string {
	sum := sha256.Sum256([]byte(descriptor))
	return hex.EncodeToString(sum[:])
}()

func Descriptor() []byte       { return []byte(descriptor) }
func Digest() string           { return compiledDigest }
func Known(digest string) bool { return digest == compiledDigest }

// MatchesResources compares compiled packaging resources with the code-owned
// descriptor, never with metadata supplied by a candidate bundle.
func MatchesResources(resources map[string][]byte) bool {
	var contract struct {
		Resources map[string]string `json:"resources"`
	}
	if json.Unmarshal([]byte(descriptor), &contract) != nil || len(resources) != len(contract.Resources) {
		return false
	}
	for name, data := range resources {
		sum := sha256.Sum256(data)
		if contract.Resources[name] != hex.EncodeToString(sum[:]) {
			return false
		}
	}
	return len(resources) == 4
}
