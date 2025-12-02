package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreds_FileNotFound(t *testing.T) {
	_, err := LoadCreds("nonexistent_file.json")

	if err == nil {
		t.Errorf("LoadCreds() with nonexistent file should return error")
	}
}

func TestLoadCreds_ValidFile(t *testing.T) {
	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "test_creds.json")

	validJSON := `{
		"credential": {
			"phoneSecret": "test_secret",
			"bsid": "test_basestation",
			"phoneId": "test_phone",
			"phonePassword": "test_phone_pass",
			"userPassword": "test_user_pass"
		}
	}`

	err := os.WriteFile(credFile, []byte(validJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to create test credentials file: %v", err)
	}

	creds, err := LoadCreds(credFile)

	if err != nil {
		t.Errorf("LoadCreds() with valid file returned error: %v", err)
	}

	if creds.Credential.PhoneSecret != "test_secret" {
		t.Errorf("LoadCreds() PhoneSecret = %q, want %q", creds.Credential.PhoneSecret, "test_secret")
	}

	if creds.Credential.BaseStation != "test_basestation" {
		t.Errorf("LoadCreds() BaseStation = %q, want %q", creds.Credential.BaseStation, "test_basestation")
	}

	if creds.Credential.Phone != "test_phone" {
		t.Errorf("LoadCreds() Phone = %q, want %q", creds.Credential.Phone, "test_phone")
	}
}

func TestLoadCreds_InvalidJSON(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "invalid_creds.json")

	invalidJSON := `{
		"credential": {
			"phoneSecret": "test_secret",
			"bsid": "test_basestation"
		} // invalid trailing comma
	}`

	err := os.WriteFile(credFile, []byte(invalidJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = LoadCreds(credFile)

	if err == nil {
		t.Errorf("LoadCreds() with invalid JSON should return error")
	}
}

func TestLoadCreds_EmptyFile(t *testing.T) {
	// Create an empty temporary file
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "empty_creds.json")

	err := os.WriteFile(credFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to create empty test file: %v", err)
	}

	_, err = LoadCreds(credFile)

	if err == nil {
		t.Errorf("LoadCreds() with empty file should return error")
	}
}

func TestLoadCreds_MalformedJSON(t *testing.T) {
	// Create a file with incomplete JSON structure
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "malformed_creds.json")

	malformedJSON := `{"credential":` // incomplete JSON

	err := os.WriteFile(credFile, []byte(malformedJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = LoadCreds(credFile)

	if err == nil {
		t.Errorf("LoadCreds() with malformed JSON should return error")
	}
}
