package secretstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	envBytemindHome = "BYTEMIND_HOME"
	defaultHomeDir  = ".bytemind"
	storeFileName   = "secrets.json"
	defaultKeyName  = "BYTEMIND_API_KEY"
	plainScheme     = "plain"
	storeVersion    = 1
)

type secretFile struct {
	Version int                    `json:"version"`
	Entries map[string]secretEntry `json:"entries"`
}

type secretEntry struct {
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
}

func Save(name, value string) error {
	keyName := normalizeKeyName(name)
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return errors.New("secret value is empty")
	}

	path, err := resolveStorePath()
	if err != nil {
		return err
	}
	payload, err := readStore(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			payload = secretFile{}
		} else {
			return err
		}
	}
	if payload.Entries == nil {
		payload.Entries = map[string]secretEntry{}
	}

	scheme, encoded, err := encodeSecret(trimmedValue)
	if err != nil {
		return err
	}
	payload.Version = storeVersion
	payload.Entries[keyName] = secretEntry{
		Scheme: scheme,
		Value:  encoded,
	}
	return writeStore(path, payload)
}

func Load(name string) (string, error) {
	keyName := normalizeKeyName(name)
	path, err := resolveStorePath()
	if err != nil {
		return "", err
	}

	payload, err := readStore(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if len(payload.Entries) == 0 {
		return "", nil
	}

	entry, ok := payload.Entries[keyName]
	if !ok {
		return "", nil
	}
	return decodeSecret(entry.Scheme, entry.Value)
}

func normalizeKeyName(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return defaultKeyName
}

func resolveStorePath() (string, error) {
	home, err := resolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "auth", storeFileName), nil
}

func resolveHomeDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(envBytemindHome)); override != "" {
		return filepath.Abs(override)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultHomeDir), nil
}

func readStore(path string) (secretFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return secretFile{}, err
	}
	var payload secretFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return secretFile{}, err
	}
	if payload.Entries == nil {
		payload.Entries = map[string]secretEntry{}
	}
	return payload, nil
}

func writeStore(path string, payload secretFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func encodeSecret(value string) (string, string, error) {
	scheme, raw, err := platformProtect([]byte(value))
	if err != nil {
		return "", "", err
	}
	return scheme, base64.StdEncoding.EncodeToString(raw), nil
}

func decodeSecret(scheme, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", err
	}
	plain, err := platformUnprotect(strings.TrimSpace(scheme), raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}
