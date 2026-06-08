package minio

import (
	"net/http"

	"github.com/minio/minio-go/v7/pkg/signer"
)

// SignRequestV4 signs an HTTP request using AWS Signature Version 4.
// Wraps minio-go/pkg/signer — replace with a native implementation when minio-go is removed.
func SignRequestV4(req http.Request, accessKey, secretKey, region string) *http.Request {
	return signer.SignV4(req, accessKey, secretKey, "", region)
}
