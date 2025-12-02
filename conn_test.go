package dd

import (
	"testing"
)

func TestSimpleRequestTarget_Constants(t *testing.T) {
	// Verify request target constants have expected values
	if DefaultTarget != 0 {
		t.Errorf("DefaultTarget = %d, want 0", DefaultTarget)
	}
	if SDKTarget != 1 {
		t.Errorf("SDKTarget = %d, want 1", SDKTarget)
	}
	if RemoteTarget != 2 {
		t.Errorf("RemoteTarget = %d, want 2", RemoteTarget)
	}
}

func TestHubSignature_Update(t *testing.T) {
	key := []byte("test_secret_key")
	hs := newHubSignature(key)

	// Test that the same inputs produce the same signature
	sig1 := hs.Update(1000, "test_data")
	sig2 := hs.Update(1000, "test_data")

	if sig1 != sig2 {
		t.Errorf("hubSignature.Update should be deterministic: sig1 = %s, sig2 = %s", sig1, sig2)
	}

	// Test that different timestamps produce different signatures
	sig3 := hs.Update(2000, "test_data")
	if sig1 == sig3 {
		t.Errorf("Different timestamps should produce different signatures")
	}

	// Test that different data produces different signatures
	sig4 := hs.Update(1000, "different_data")
	if sig1 == sig4 {
		t.Errorf("Different data should produce different signatures")
	}

	// Verify signature is base64 encoded (should not be empty and contain valid chars)
	if len(sig1) == 0 {
		t.Errorf("Signature should not be empty")
	}
}

func TestMd5hash(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Empty string", ""},
		{"Simple string", "hello"},
		{"Numeric string", "12345"},
		{"Special chars", "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := md5hash(tt.input)

			// MD5 hash should always be 16 bytes
			if len(hash) != 16 {
				t.Errorf("md5hash(%q) length = %d, want 16", tt.input, len(hash))
			}

			// Same input should produce same hash
			hash2 := md5hash(tt.input)
			if string(hash) != string(hash2) {
				t.Errorf("md5hash should be deterministic")
			}
		})
	}
}

func TestDataPayload_readData_Unencrypted(t *testing.T) {
	dp := dataPayload{
		IsEncrypted: false,
		Data:        "test_plaintext_data",
	}

	key := []byte("dummy_key")
	result, err := dp.readData(key)

	if err != nil {
		t.Errorf("readData() with unencrypted data should not error: %v", err)
	}

	if string(result) != "test_plaintext_data" {
		t.Errorf("readData() = %q, want %q", string(result), "test_plaintext_data")
	}
}

func TestDataPayload_readData_InvalidBase64(t *testing.T) {
	dp := dataPayload{
		IsEncrypted: true,
		Time:        1000,
		Data:        "!!!invalid-base64!!!",
	}

	// Use a valid 16-byte key for AES
	key := make([]byte, 16)

	_, err := dp.readData(key)

	if err == nil {
		t.Errorf("readData() with invalid base64 should return error")
	}
}

func TestPKCS5Padding(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		blockSize int
		wantLen   int
	}{
		{"Empty input", []byte{}, 16, 16},
		{"15 bytes", make([]byte, 15), 16, 16},
		{"16 bytes", make([]byte, 16), 16, 32},
		{"17 bytes", make([]byte, 17), 16, 32},
		{"8 byte block", make([]byte, 5), 8, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PKCS5Padding(tt.input, tt.blockSize)

			if len(result) != tt.wantLen {
				t.Errorf("PKCS5Padding(%d bytes, blockSize %d) length = %d, want %d",
					len(tt.input), tt.blockSize, len(result), tt.wantLen)
			}

			// Verify padding value is correct
			if len(result) > 0 {
				paddingValue := result[len(result)-1]
				expectedPadding := byte(tt.blockSize - (len(tt.input) % tt.blockSize))
				if paddingValue != expectedPadding {
					t.Errorf("Last byte (padding value) = %d, want %d", paddingValue, expectedPadding)
				}
			}
		})
	}
}

func TestPKCS5Trimming(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantLen int
	}{
		{"Valid padding of 1", []byte{1, 2, 3, 4, 5, 1}, 5},
		{"Valid padding of 2", []byte{1, 2, 3, 4, 2, 2}, 4},
		{"Valid padding of 5", []byte{1, 2, 3, 5, 5, 5, 5, 5}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PKCS5Trimming(tt.input)

			if len(result) != tt.wantLen {
				t.Errorf("PKCS5Trimming() length = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestPKCS5Trimming_InvalidPadding(t *testing.T) {
	// Test with invalid padding (padding value exceeds length)
	invalidInput := []byte{1, 2, 3, 100}
	result := PKCS5Trimming(invalidInput)

	// Should return original input when padding is invalid
	if len(result) != len(invalidInput) {
		t.Errorf("PKCS5Trimming with invalid padding should return original input")
	}
}

func TestNewEncCipher_InvalidKeyLength(t *testing.T) {
	// AES requires key length of 16, 24, or 32 bytes
	invalidKey := []byte("short")

	_, err := NewEncCipher(invalidKey, 1000)

	if err == nil {
		t.Errorf("NewEncCipher() with invalid key length should return error")
	}
}

func TestNewDecCipher_InvalidKeyLength(t *testing.T) {
	// AES requires key length of 16, 24, or 32 bytes
	invalidKey := []byte("short")

	_, err := NewDecCipher(invalidKey, 1000)

	if err == nil {
		t.Errorf("NewDecCipher() with invalid key length should return error")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	// Test that encryption followed by decryption returns original data
	key := make([]byte, 16) // 16-byte key for AES-128
	for i := range key {
		key[i] = byte(i)
	}

	timestamp := 1000
	plaintext := []byte("Hello, World! This is a test message.")

	// Encrypt
	encCipher, err := NewEncCipher(key, timestamp)
	if err != nil {
		t.Fatalf("NewEncCipher() error = %v", err)
	}
	ciphertext := encCipher.Encrypt(plaintext)

	// Decrypt
	decCipher, err := NewDecCipher(key, timestamp)
	if err != nil {
		t.Fatalf("NewDecCipher() error = %v", err)
	}
	decrypted := decCipher.Decrypt(ciphertext)

	// Compare
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt(Encrypt(%q)) = %q, want original plaintext", plaintext, decrypted)
	}
}
