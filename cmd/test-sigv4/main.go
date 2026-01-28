package main

import (
	"fmt"
	"net/http"

	"github.com/mallardduck/dirio/internal/auth"
)

func main() {
	// Recreate the exact request from AWS CLI
	req, _ := http.NewRequest("GET", "http://localhost:19000/", nil)

	// Set the exact headers from AWS CLI output
	req.Header.Set("Host", "localhost:19000")
	req.Header.Set("X-Amz-Date", "20260124T020429Z")
	req.Header.Set("X-Amz-Content-SHA256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=testaccess/20260124/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=f6e58d81aa08f048375097f41e64a988191929de3877d2e9e594d4e0ad491b77")

	secretKey := "testsecret"

	// Try to verify with debug output
	fmt.Println("Testing AWS CLI request...")
	err := auth.DebugVerifySignature(req, secretKey)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Println("SUCCESS!")
	}
}