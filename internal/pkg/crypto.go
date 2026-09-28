package pkg

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// NewMasterKey 生成 32 字节主密钥（Base64 编码后落配置）。
func NewMasterKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", Wrap(2001, "生成主密钥失败", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// Encrypt 用 AES-256-GCM 加密，输出 Base64(nonce|ciphertext)。
func Encrypt(masterKeyB64, plain string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return "", New(2002, "主密钥格式不正确", "")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", Wrap(2002, "主密钥长度不正确", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", Wrap(2002, "初始化加密器失败", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", Wrap(2003, "生成随机 nonce 失败", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解 Encrypt 的输出；密钥错误或数据被篡改时返回错误。
func Decrypt(masterKeyB64, sealedB64 string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return "", New(2002, "主密钥格式不正确", "")
	}
	raw, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return "", New(2004, "密文格式不正确", "")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", Wrap(2002, "主密钥长度不正确", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", Wrap(2002, "初始化加密器失败", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", New(2004, "密文长度不足", "")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", Wrap(2005, "解密失败，密文与主密钥不匹配", err)
	}
	return string(plain), nil
}
