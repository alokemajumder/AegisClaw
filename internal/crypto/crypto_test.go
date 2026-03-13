package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "simple string", plaintext: "hello world"},
		{name: "empty string", plaintext: ""},
		{name: "binary-like data", plaintext: "\x00\x01\x02\xff\xfe"},
		{name: "unicode", plaintext: "security validation platform"},
		{name: "long payload", plaintext: string(make([]byte, 4096))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateKey()
			require.NoError(t, err)

			ciphertext, err := Encrypt([]byte(tt.plaintext), key)
			require.NoError(t, err)
			assert.NotEqual(t, []byte(tt.plaintext), ciphertext, "ciphertext should differ from plaintext")

			decrypted, err := Decrypt(ciphertext, key)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, string(decrypted))
		})
	}
}

func TestEncrypt_ProducesDifferentCiphertexts(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	plaintext := []byte("same plaintext")
	ct1, err := Encrypt(plaintext, key)
	require.NoError(t, err)
	ct2, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	assert.NotEqual(t, ct1, ct2, "repeated encryptions should produce different ciphertexts due to random nonce")
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	plaintext := []byte("sensitive data")
	ciphertext, err := Encrypt(plaintext, key1)
	require.NoError(t, err)

	_, err = Decrypt(ciphertext, key2)
	assert.Error(t, err, "decrypting with wrong key should fail")
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	ciphertext, err := Encrypt([]byte("original"), key)
	require.NoError(t, err)

	// Flip a byte in the ciphertext payload (after the nonce)
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xff

	_, err = Decrypt(tampered, key)
	assert.Error(t, err, "decrypting tampered ciphertext should fail")
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	_, err = Decrypt([]byte("short"), key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	badKey := []byte("too-short")
	_, err := Encrypt([]byte("data"), badKey)
	assert.Error(t, err, "encrypt with non-32-byte key should fail")
}

func TestHMACSHA256_AndVerify_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "simple", data: "hello"},
		{name: "empty", data: ""},
		{name: "json payload", data: `{"action":"validate","tier":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateKey()
			require.NoError(t, err)

			sig := HMACSHA256([]byte(tt.data), key)
			assert.NotEmpty(t, sig)
			assert.Len(t, sig, 64, "HMAC-SHA256 hex digest should be 64 chars")

			ok := VerifyHMAC([]byte(tt.data), key, sig)
			assert.True(t, ok, "HMAC verification should pass for correct data and key")
		})
	}
}

func TestHMACSHA256_Deterministic(t *testing.T) {
	key := []byte("fixed-test-key-32-bytes-long!!!!!")
	data := []byte("deterministic check")

	sig1 := HMACSHA256(data, key)
	sig2 := HMACSHA256(data, key)
	assert.Equal(t, sig1, sig2, "same data+key should produce same HMAC")
}

func TestVerifyHMAC_WrongData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	sig := HMACSHA256([]byte("original data"), key)

	ok := VerifyHMAC([]byte("tampered data"), key, sig)
	assert.False(t, ok, "HMAC verification should fail for wrong data")
}

func TestVerifyHMAC_WrongKey(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	data := []byte("test data")
	sig := HMACSHA256(data, key1)

	ok := VerifyHMAC(data, key2, sig)
	assert.False(t, ok, "HMAC verification should fail for wrong key")
}

func TestVerifyHMAC_InvalidSignature(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	ok := VerifyHMAC([]byte("data"), key, "not-a-valid-hex-signature")
	assert.False(t, ok)
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)
	assert.Len(t, key, 32, "generated key should be 32 bytes for AES-256")
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2, "two generated keys should not be equal")
}

func TestHashSHA256_Consistency(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "simple", data: "hello"},
		{name: "empty", data: ""},
		{name: "binary", data: "\x00\x01\x02"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1 := HashSHA256([]byte(tt.data))
			h2 := HashSHA256([]byte(tt.data))

			assert.Equal(t, h1, h2, "same input should always produce same hash")
			assert.Len(t, h1, 64, "SHA-256 hex digest should be 64 chars")
		})
	}
}

func TestHashSHA256_DifferentInputs(t *testing.T) {
	h1 := HashSHA256([]byte("input one"))
	h2 := HashSHA256([]byte("input two"))

	assert.NotEqual(t, h1, h2, "different inputs should produce different hashes")
}

func TestHashSHA256_KnownVector(t *testing.T) {
	// SHA-256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	hash := HashSHA256([]byte(""))
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", hash)
}
