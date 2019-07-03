package dd

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"
)

type cbcCipher struct {
	block cipher.Block
	cb    cipher.BlockMode
}

type cbcEncCipher struct {
	cbcCipher
}

func NewEncCipher(key []byte, t int) (*cbcEncCipher, error) {
	out := &cbcEncCipher{}
	var err error

	out.block, err = aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := md5hash(fmt.Sprintf("%d", t))
	out.cb = cipher.NewCBCEncrypter(out.block, iv)
	return out, nil
}

func (c *cbcEncCipher) Encrypt(src []byte) []byte {
	content := PKCS5Padding(src, c.block.BlockSize())
	crypted := make([]byte, len(content))
	c.cb.CryptBlocks(crypted, content)
	return crypted
}

type cbcDecCipher struct {
	cbcCipher
}

func NewDecCipher(key []byte, t int) (*cbcDecCipher, error) {
	out := &cbcDecCipher{}
	var err error

	out.block, err = aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := md5hash(fmt.Sprintf("%d", t))
	out.cb = cipher.NewCBCDecrypter(out.block, iv)
	return out, nil
}

func (c *cbcDecCipher) Decrypt(src []byte) []byte {
	decrypted := make([]byte, len(src))
	c.cb.CryptBlocks(decrypted, src)
	return PKCS5Trimming(decrypted)
}

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding) // pad with length of padding
	return append(ciphertext, padtext...)
}

func PKCS5Trimming(encrypt []byte) []byte {
	padding := encrypt[len(encrypt)-1]
	if int(padding) > len(encrypt) || int(padding) <= 0 {
		log.Printf("badly encoded CBC padding: %v (enc=%+v)", padding, encrypt)
		return encrypt
	}
	return encrypt[:len(encrypt)-int(padding)]
}

func renderTime(t time.Time) int64 {
	return t.UnixNano() / 1e6
}

func md5hash(s string) []byte {
	h := md5.New()
	io.WriteString(h, s)
	return h.Sum(nil)
}

// gets instance of HmacSHA256 "Mac"
// creates new SecretKeySpec: passed bytes, "HMACSHA256"

type hubSignature struct {
	key []byte
}

func (hs *hubSignature) Update(t int, data string) string {
	// badly named, as we just need to sign anew every time
	h := hmac.New(sha256.New, hs.key)

	s := fmt.Sprintf("%d:%s", t, data)
	h.Write([]byte(s))
	b := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(b)
}

func newHubSignature(key []byte) *hubSignature {
	return &hubSignature{
		key: key,
	}
}

// dataPayload optionally contains encrypted data.
type dataPayload struct {
	IsEncrypted bool   `json:"isEncrypted,omitempty"`
	Time        int    `json:"time,omitempty"`
	Data        string `json:"data,omitempty"`
}

// readData reads this dataPayload, transparently decrypting if required.
func (dp *dataPayload) readData(key []byte) ([]byte, error) {
	if !dp.IsEncrypted {
		return []byte(dp.Data), nil
	}

	c, err := NewDecCipher(key, dp.Time)
	if err != nil {
		return nil, err
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(dp.Data)
	if err != nil {
		return nil, err
	}
	return c.Decrypt(cipherBytes), nil
}

// unmarshalData is a convenience over readData, which unmarshals the payload via JSON.
func (dp *dataPayload) unmarshalData(key []byte, target interface{}) error {
	b, err := dp.readData(key)
	if err != nil {
		return err
	} else if len(b) == 0 {
		return errors.New("no data to unmarshal from payload")
	}
	return json.Unmarshal(b, target)
}
