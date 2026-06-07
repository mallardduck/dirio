package http

import (
	"encoding/json"
	"io"

	"github.com/minio/madmin-go/v3"
)

// decryptAndUnmarshal decrypts a madmin-encrypted payload from r and unmarshals it into v.
// All MinIO Admin API request bodies use this encryption scheme.
func decryptAndUnmarshal(secretKey string, r io.Reader, v any) error {
	decrypted, err := madmin.DecryptData(secretKey, r)
	if err != nil {
		return err
	}
	return json.Unmarshal(decrypted, v)
}

// marshalAndEncrypt marshals v to JSON and encrypts it using the madmin wire protocol.
// All MinIO Admin API encrypted responses use this scheme.
func marshalAndEncrypt(secretKey string, v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return madmin.EncryptData(secretKey, data)
}
