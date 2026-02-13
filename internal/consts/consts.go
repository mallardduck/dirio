package consts

const (
	DefaultBucketLocation = "us-east-1"
)

// AWS Signature V4 Headers
const (
	// HeaderContentSHA256 is the AWS SigV4 header for payload hash
	HeaderContentSHA256 = "X-Amz-Content-Sha256"

	// HeaderDate is the AWS SigV4 header for request timestamp
	HeaderDate = "X-Amz-Date"

	// HeaderCopySource is the S3 header for copy operations
	HeaderCopySource = "X-Amz-Copy-Source"

	// HeaderBucketRegion is the S3 header for bucket region
	HeaderBucketRegion = "x-amz-bucket-region"

	// ContentSHA256Streaming is the value for chunked transfer encoding
	ContentSHA256Streaming = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"

	// ContentSHA256Unsigned is the value for unsigned payloads
	ContentSHA256Unsigned = "UNSIGNED-PAYLOAD"
)
