package evidence

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFilename_Valid(t *testing.T) {
	tests := []string{
		"report.json",
		"evidence-capture.pcap",
		"log_2024-01-01.txt",
		"a",
	}
	for _, name := range tests {
		assert.NoError(t, validateFilename(name), "expected valid: %s", name)
	}
}

func TestValidateFilename_Empty(t *testing.T) {
	err := validateFilename("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestValidateFilename_PathTraversal(t *testing.T) {
	tests := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"foo/../bar",
	}
	for _, name := range tests {
		err := validateFilename(name)
		assert.Error(t, err, "expected error for: %s", name)
		assert.Contains(t, err.Error(), "..")
	}
}

func TestValidateFilename_PathSeparators(t *testing.T) {
	tests := []string{
		"path/to/file",
		"path\\to\\file",
	}
	for _, name := range tests {
		err := validateFilename(name)
		assert.Error(t, err, "expected error for: %s", name)
		assert.Contains(t, err.Error(), "path separators")
	}
}

func TestValidateFilename_TooLong(t *testing.T) {
	name := strings.Repeat("a", 256)
	err := validateFilename(name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "255")
}

func TestValidateFilename_MaxLength(t *testing.T) {
	name := strings.Repeat("a", 255)
	assert.NoError(t, validateFilename(name))
}

func TestAllowedContentTypes(t *testing.T) {
	allowed := []string{
		"application/json",
		"text/plain",
		"image/png",
		"application/octet-stream",
	}
	for _, ct := range allowed {
		assert.True(t, allowedContentTypes[ct], "expected allowed: %s", ct)
	}

	disallowed := []string{
		"application/javascript",
		"text/javascript",
		"application/x-executable",
		"video/mp4",
	}
	for _, ct := range disallowed {
		assert.False(t, allowedContentTypes[ct], "expected disallowed: %s", ct)
	}
}

func TestUploadSizeLimits(t *testing.T) {
	assert.Equal(t, 100<<20, MaxUploadSize, "MaxUploadSize should be 100MB")
	assert.Equal(t, 10<<20, MaxReceiptSize, "MaxReceiptSize should be 10MB")
}
