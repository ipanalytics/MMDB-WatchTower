package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CosignBlob(ctx context.Context, blobPath, keyPath string, signature []byte) error {
	if len(signature) == 0 {
		return nil
	}
	if keyPath == "" {
		return fmt.Errorf("cosign_key is required when signature_url is configured")
	}
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign executable not found: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(blobPath), "mmdbwatch-*.sig")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(signature); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	defer os.Remove(tmp.Name())
	cmd := exec.CommandContext(ctx, "cosign", "verify-blob", "--key", keyPath, "--signature", tmp.Name(), blobPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cosign verify-blob failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
