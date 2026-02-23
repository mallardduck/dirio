package response

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/global"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// WriteXMLResponse writes an HTTP response in XML format with the XML declaration header.
// It buffers the encoded output before writing so that headers are only sent on success.
func WriteXMLResponse(w http.ResponseWriter, statusCode int, data any) error {
	var buf bytes.Buffer
	buf.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	defer func() { _ = encoder.Flush() }()

	if err := encoder.Encode(data); err != nil {
		return err
	}

	if err := encoder.Flush(); err != nil {
		return err
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(statusCode)

	_, err := w.Write(buf.Bytes())
	return err
}

type ErrorModifier func(errResponse s3types.ErrorResponse) s3types.ErrorResponse

func SetErrAsMessage(err error) ErrorModifier {
	return func(errResponse s3types.ErrorResponse) s3types.ErrorResponse {
		if err != nil {
			errResponse.Message = err.Error()
		}

		return errResponse
	}
}

type XMLErrorWriter func(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, additionalErrorCallbacks ...ErrorModifier) error

var _ XMLErrorWriter = WriteErrorResponse

// WriteErrorResponse writes an S3-compatible XML error response.
// The HTTP status code is derived from errCode. If err is non-nil, its message
// overrides the error code's default description.
func WriteErrorResponse(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, additionalErrorCallbacks ...ErrorModifier) error {
	msg := errCode.Description()

	errResponse := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   msg,
		RequestID: requestID,
		HostID:    global.GlobalInstanceID().String(),
	}

	// If no modifiers are provided, the loop simply doesn't run
	for _, modify := range additionalErrorCallbacks {
		if modify != nil {
			errResponse = modify(errResponse)
		}
	}

	if writeErr := WriteXMLResponse(w, errCode.HTTPStatus(), errResponse); writeErr != nil {
		return fmt.Errorf("failed to encode error response: %w", writeErr)
	}

	return nil
}
