package weixin

import (
	"bytes"
	"crypto/aes"
	"testing"
)

func TestDecryptAESECB(t *testing.T) {
	key := []byte("0123456789abcdef")
	plain := []byte("hello image")
	encrypted := encryptAESECBForTest(t, padPKCS7ForTest(plain, aes.BlockSize), key)

	got, err := decryptAESECB(encrypted, key)
	if err != nil {
		t.Fatalf("decryptAESECB() error = %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decryptAESECB() = %q, want %q", got, plain)
	}
}

func encryptAESECBForTest(t *testing.T, content, key []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := make([]byte, len(content))
	for offset := 0; offset < len(content); offset += aes.BlockSize {
		block.Encrypt(encrypted[offset:offset+aes.BlockSize], content[offset:offset+aes.BlockSize])
	}
	return encrypted
}

func padPKCS7ForTest(content []byte, blockSize int) []byte {
	padding := blockSize - len(content)%blockSize
	return append(append([]byte(nil), content...), bytes.Repeat([]byte{byte(padding)}, padding)...)
}
