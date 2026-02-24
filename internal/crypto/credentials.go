package crypto

import (
	"crypto/rand"
	"math/big"
)

// KeyPrefix defines our custom enum type for different access levels
type KeyPrefix string

const (
	// PrefixUser represents standard robotic entity access
	PrefixUser KeyPrefix = "DIOA"
	// PrefixSystem represents internal system/root access
	PrefixSystem KeyPrefix = "DIOR"
	// PrefixService represents automated service-to-service access
	PrefixService KeyPrefix = "DIOS"
)

const (
	accessKeyLength = 16 // 4 (prefix) + 16 = 20 total
	secretKeyLength = 40
	charset         = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	secretCharset   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// GenerateDirIOKey creates a formatted Access Key and Secret Key pair
func GenerateDirIOKey(prefix KeyPrefix) (accessKey, secretKey string, err error) {
	accessKey, err = generateRandomString(string(prefix), charset, accessKeyLength)
	if err != nil {
		return "", "", err
	}

	secretKey, err = generateRandomString("", secretCharset, secretKeyLength)
	if err != nil {
		return "", "", err
	}

	return accessKey, secretKey, nil
}

func generateRandomString(prefix, alphabet string, length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		result[i] = alphabet[num.Int64()]
	}
	return prefix + string(result), nil
}
