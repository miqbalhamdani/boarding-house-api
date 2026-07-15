// Package storage provides a private object-storage abstraction for
// payment-proof files (Module 10). Proof binaries are never stored in
// PostgreSQL; only the object key is persisted. Viewers receive short-lived
// presigned URLs rather than public links.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxProofFileSize bounds a single uploaded proof file (5 MB).
const MaxProofFileSize = 5 * 1024 * 1024

// Validation errors, mapped by callers to VALIDATION_ERROR responses.
var (
	ErrUnsupportedContentType = errors.New("unsupported file type: only JPEG, PNG and WebP images are allowed")
	ErrFileTooLarge           = errors.New("file exceeds the 5MB maximum size")
	ErrEmptyFile              = errors.New("file is empty")
)

// allowedProofContentTypes maps an accepted sniffed MIME type to the file
// extension used when building the object key. This is an allow-list: any
// content type not present here (SVG, executables, octet-stream, …) is
// rejected by default.
var allowedProofContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// Store is the private object-storage dependency Module 10 relies on.
type Store interface {
	// Put uploads content under key with the given content type.
	Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error
	// PresignGet returns a short-lived signed URL for viewing the object.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Remove best-effort deletes an object (used to clean up an orphaned row
	// when an upload fails after the DB insert).
	Remove(ctx context.Context, key string) error
}

// DetectAndValidate sniffs the real content type from the file bytes (never the
// client-supplied filename or Content-Type header) and validates size and type.
// It returns the extension and canonical content type to use for the object.
//
// Content sniffing is the single enforcement point for "verify file content,
// not filename" and "reject SVG and executables": because the check is an
// allow-list, anything that does not sniff to an accepted image type is
// rejected regardless of how it is labelled.
func DetectAndValidate(content []byte) (ext string, contentType string, err error) {
	if len(content) == 0 {
		return "", "", ErrEmptyFile
	}
	if len(content) > MaxProofFileSize {
		return "", "", ErrFileTooLarge
	}

	ct := detectContentType(content)
	ext, ok := allowedProofContentTypes[ct]
	if !ok {
		return "", "", ErrUnsupportedContentType
	}
	return ext, ct, nil
}

// detectContentType returns the sniffed MIME type. It falls back to a RIFF/WEBP
// magic-byte check because Go's net/http.DetectContentType does not recognise
// WebP on all versions (it returns application/octet-stream instead).
func detectContentType(content []byte) string {
	if isWebP(content) {
		return "image/webp"
	}
	return http.DetectContentType(content)
}

// isWebP reports whether content begins with the RIFF....WEBP container header.
func isWebP(content []byte) bool {
	return len(content) >= 12 &&
		string(content[0:4]) == "RIFF" &&
		string(content[8:12]) == "WEBP"
}

// ProofObjectKey builds the server-generated object key. The tenant's original
// filename is never used.
//
//	owners/{owner_id}/payment-proofs/{submission_id}/proof.{ext}
func ProofObjectKey(ownerID, submissionID, ext string) string {
	return fmt.Sprintf("owners/%s/payment-proofs/%s/proof.%s", ownerID, submissionID, ext)
}
