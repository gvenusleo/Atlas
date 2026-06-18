package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const maxWeixinImageBytes = 20 * 1024 * 1024

// imagePartFromItem 下载微信图片并转换为模型图片片段。
func (s *Server) imagePartFromItem(ctx context.Context, item MessageItem) (model.ContentPart, error) {
	if item.Type != messageItemTypeImage || item.ImageItem == nil {
		return model.ContentPart{}, fmt.Errorf("weixin image item is required")
	}
	media := item.ImageItem.Media
	if media == nil {
		media = item.ImageItem.ThumbMedia
	}
	if media == nil {
		return model.ContentPart{}, fmt.Errorf("weixin image media is required")
	}
	content, err := s.downloadMedia(ctx, media, item.ImageItem.AESKey)
	if err != nil {
		return model.ContentPart{}, err
	}
	mimeType := http.DetectContentType(content)
	if !strings.HasPrefix(mimeType, "image/") {
		return model.ContentPart{}, fmt.Errorf("weixin media is not an image: %s", mimeType)
	}
	return model.ContentPart{
		Type:     model.ContentPartImage,
		MimeType: mimeType,
		DataURL:  "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(content),
		Detail:   model.ImageDetailAuto,
	}, nil
}

func (s *Server) downloadMedia(ctx context.Context, media *CDNMedia, rawAESKey string) ([]byte, error) {
	target, err := s.mediaURL(media)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("weixin media download failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxWeixinImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxWeixinImageBytes {
		return nil, fmt.Errorf("weixin image exceeds %d bytes", maxWeixinImageBytes)
	}
	key, ok, err := mediaAESKey(media, rawAESKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return content, nil
	}
	return decryptAESECB(content, key)
}

func (s *Server) mediaURL(media *CDNMedia) (string, error) {
	if strings.TrimSpace(media.FullURL) != "" {
		return media.FullURL, nil
	}
	query := strings.TrimSpace(media.EncryptQueryParam)
	if query == "" {
		return "", fmt.Errorf("weixin media full_url or encrypt_query_param is required")
	}
	return strings.TrimRight(s.cdnBaseURL, "/") + "/download?encrypted_query_param=" + url.QueryEscape(query), nil
}

func mediaAESKey(media *CDNMedia, rawAESKey string) ([]byte, bool, error) {
	if strings.TrimSpace(rawAESKey) != "" {
		key, err := hex.DecodeString(strings.TrimSpace(rawAESKey))
		if err != nil || len(key) != aes.BlockSize {
			return nil, false, fmt.Errorf("weixin image aeskey is invalid")
		}
		return key, true, nil
	}
	if strings.TrimSpace(media.AESKey) == "" {
		return nil, false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(media.AESKey)
	if err != nil {
		return nil, false, fmt.Errorf("weixin media aes_key is invalid")
	}
	if len(decoded) == aes.BlockSize {
		return decoded, true, nil
	}
	if len(decoded) == aes.BlockSize*2 {
		key, err := hex.DecodeString(string(decoded))
		if err == nil && len(key) == aes.BlockSize {
			return key, true, nil
		}
	}
	return nil, false, fmt.Errorf("weixin media aes_key is invalid")
}

func decryptAESECB(content, key []byte) ([]byte, error) {
	if len(content) == 0 || len(content)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin encrypted media has invalid size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(content))
	for offset := 0; offset < len(content); offset += aes.BlockSize {
		block.Decrypt(plain[offset:offset+aes.BlockSize], content[offset:offset+aes.BlockSize])
	}
	return unpadPKCS7(plain, aes.BlockSize)
}

func unpadPKCS7(content []byte, blockSize int) ([]byte, error) {
	if len(content) == 0 || len(content)%blockSize != 0 {
		return nil, fmt.Errorf("invalid PKCS7 content size")
	}
	padding := int(content[len(content)-1])
	if padding == 0 || padding > blockSize || padding > len(content) {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	if !bytes.Equal(content[len(content)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	return content[:len(content)-padding], nil
}
