package s3types

import "encoding/xml"

// ListBucketsResponse is the response for ListBuckets operation
type ListBucketsResponse struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListAllMyBucketsResult"`
	Owner   Owner    `xml:"Owner"`
	Buckets []Bucket `xml:"Buckets>Bucket"`
}

// ListBucketResult is the response for ListObjects (V1)
type ListBucketResult struct {
	XMLName        xml.Name       `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name           string         `xml:"Name"`
	Prefix         string         `xml:"Prefix"`
	Marker         string         `xml:"Marker"`
	NextMarker     string         `xml:"NextMarker,omitempty"`
	Delimiter      string         `xml:"Delimiter,omitempty"`
	MaxKeys        int            `xml:"MaxKeys"`
	IsTruncated    bool           `xml:"IsTruncated"`
	Contents       []Object       `xml:"Contents"`
	CommonPrefixes []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// ListBucketV2Result is the response for ListObjectsV2
type ListBucketV2Result struct {
	XMLName               xml.Name       `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	KeyCount              int            `xml:"KeyCount"`
	IsTruncated           bool           `xml:"IsTruncated"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	StartAfter            string         `xml:"StartAfter,omitempty"`
	Contents              []Object       `xml:"Contents"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// LocationResponse is the response for GetBucketLocation
type LocationResponse struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ LocationConstraint"`
	Location string   `xml:",chardata"`
}

// DeleteObjectsRequest is the request body for DeleteObjects operation
type DeleteObjectsRequest struct {
	XMLName xml.Name           `xml:"Delete"`
	Quiet   bool               `xml:"Quiet,omitempty"`
	Objects []ObjectIdentifier `xml:"Object"`
}

// DeleteObjectsResult is the response for DeleteObjects operation
type DeleteObjectsResult struct {
	XMLName xml.Name        `xml:"http://s3.amazonaws.com/doc/2006-03-01/ DeleteResult"`
	Deleted []DeletedObject `xml:"Deleted,omitempty"`
	Errors  []DeleteError   `xml:"Error,omitempty"`
}

// CopyObjectResult is the response for CopyObject operation
type CopyObjectResult struct {
	XMLName      xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

// InitiateMultipartUploadResult is the response for CreateMultipartUpload operation
type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

// CompleteMultipartUploadResult is the response for CompleteMultipartUpload operation
type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

// CompleteMultipartUpload is the request body for CompleteMultipartUpload operation
type CompleteMultipartUpload struct {
	XMLName xml.Name       `xml:"CompleteMultipartUpload"`
	Parts   []CompletePart `xml:"Part"`
}

// CompletePart represents a part in the CompleteMultipartUpload request
type CompletePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// ListPartsResult is the response for ListParts operation
type ListPartsResult struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListPartsResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
	Parts    []Part   `xml:"Part"`
}

// Part represents a single part in ListPartsResult
type Part struct {
	PartNumber   int    `xml:"PartNumber"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	LastModified string `xml:"LastModified"`
}
