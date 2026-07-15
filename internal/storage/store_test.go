package storage

import (
	"errors"
	"strings"
	"testing"
)

// Minimal valid file headers for content sniffing.
var (
	jpegHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	pngHeader  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
)

func webpBytes() []byte {
	b := make([]byte, 16)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	return b
}

func TestDetectAndValidate_AcceptsJPEGPNGWebp(t *testing.T) {
	cases := map[string]struct {
		content []byte
		wantExt string
		wantCT  string
	}{
		"jpeg": {jpegHeader, "jpg", "image/jpeg"},
		"png":  {pngHeader, "png", "image/png"},
		"webp": {webpBytes(), "webp", "image/webp"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ext, ct, err := DetectAndValidate(tc.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext != tc.wantExt || ct != tc.wantCT {
				t.Fatalf("got ext=%q ct=%q, want ext=%q ct=%q", ext, ct, tc.wantExt, tc.wantCT)
			}
		})
	}
}

func TestDetectAndValidate_RejectsSVGByContentSniffing(t *testing.T) {
	// SVG content sniffs to text/xml, not an allowed image type — rejected even
	// though a caller might have named it "proof.jpg".
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if _, _, err := DetectAndValidate(svg); !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("want ErrUnsupportedContentType, got %v", err)
	}
}

func TestDetectAndValidate_RejectsExecutable(t *testing.T) {
	// ELF magic bytes — not an image, must be rejected by the allow-list.
	elf := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if _, _, err := DetectAndValidate(elf); !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("want ErrUnsupportedContentType, got %v", err)
	}
}

func TestDetectAndValidate_RejectsOversized(t *testing.T) {
	big := make([]byte, MaxProofFileSize+1)
	copy(big, jpegHeader)
	if _, _, err := DetectAndValidate(big); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestDetectAndValidate_RejectsEmpty(t *testing.T) {
	if _, _, err := DetectAndValidate(nil); !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("want ErrEmptyFile, got %v", err)
	}
}

func TestProofObjectKey_Shape(t *testing.T) {
	key := ProofObjectKey("owner-1", "sub-1", "jpg")
	want := "owners/owner-1/payment-proofs/sub-1/proof.jpg"
	if key != want {
		t.Fatalf("got %q, want %q", key, want)
	}
	if !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("key should end with the extension: %q", key)
	}
}
