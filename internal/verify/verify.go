package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func Size(path string, minMB, maxMB int64) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := st.Size()
	if minMB > 0 && size < minMB*1024*1024 {
		return fmt.Errorf("file too small: %d bytes", size)
	}
	if maxMB > 0 && size > maxMB*1024*1024 {
		return fmt.Errorf("file too large: %d bytes", size)
	}
	return nil
}

func SHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func SHA256Matches(path, expectedText string) (string, error) {
	actual, err := SHA256(path)
	if err != nil {
		return "", err
	}
	expected := strings.Fields(expectedText)
	if len(expected) == 0 {
		return "", fmt.Errorf("sha256 response is empty")
	}
	if !strings.EqualFold(actual, expected[0]) {
		return actual, fmt.Errorf("sha256 mismatch: expected %s got %s", expected[0], actual)
	}
	return actual, nil
}
