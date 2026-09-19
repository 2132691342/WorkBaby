// 文件：AES-GCM 加解密（API Key / SMTP 密码 / Webhook token）。

package pkg

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Cipher 持有加密主密钥（32 字节 AES-256）；首启从 config.yaml 加载或随机生成。
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipherFromB64 从 base64 字符串恢复主密钥；空字符串返回 error（说明 config 未配）。
func NewCipherFromB64(b64 string) (*Cipher, error) {
	if b64 == "" {
		return nil, New(2020, "master key not configured", "")
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, Wrap(2021, "invalid master key encoding", err)
	}
	if len(key) != 32 {
		return nil, New(2022, "master key length must be 32 bytes (AES-256)", "")
	}
	return newCipherFromKey(key)
}

// NewCipherRandom 生成一次性随机密钥；用于首次启动写入 config.yaml。
func NewCipherRandom() (*Cipher, string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, "", Wrap(2023, "generate random key failed", err)
	}
	c, err := newCipherFromKey(key)
	if err != nil {
		return nil, "", err
	}
	return c, base64.StdEncoding.EncodeToString(key), nil
}

func newCipherFromKey(key []byte) (*Cipher, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, Wrap(2024, "init AES cipher failed", err)
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, Wrap(2025, "init GCM mode failed", err)
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt 加密明文 → base64(nonce ‖ ciphertext) 字符串；空串返回空串（避免误写入）。
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if c == nil {
		return "", New(2026, "cipher not initialized", "")
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", Wrap(2027, "generate nonce failed", err)
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt 反向：base64 → 明文；空串返回空串。
func (c *Cipher) Decrypt(b64 string) (string, error) {
	if b64 == "" {
		return "", nil
	}
	if c == nil {
		return "", New(2026, "cipher not initialized", "")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", Wrap(2028, "invalid base64 ciphertext", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", New(2029, "ciphertext too short", "")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", Wrap(2029, "decrypt failed (wrong key or tampering)", err)
	}
	return string(pt), nil
}

// ErrCipherNotInit 哨兵错误，便于 service 层判断是否需要退化为明文（用户未配密钥时）。
var ErrCipherNotInit = errors.New("cipher not initialized")
