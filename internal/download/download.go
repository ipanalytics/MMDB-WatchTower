package download

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Request struct {
	URL         string
	Headers     map[string]string
	Dir         string
	IfNoneMatch string
	TLS         TLS
}

type TLS struct {
	ClientCert string
	ClientKey  string
	CACert     string
	ServerName string
}

type Result struct {
	Path        string
	ETag        string
	NotModified bool
}

func File(ctx context.Context, req Request) (Result, error) {
	if req.Dir == "" {
		req.Dir = os.TempDir()
	}
	if err := os.MkdirAll(req.Dir, 0755); err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return Result{}, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if req.IfNoneMatch != "" {
		httpReq.Header.Set("If-None-Match", req.IfNoneMatch)
	}
	client, err := client(req.TLS)
	if err != nil {
		return Result{}, err
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return Result{ETag: req.IfNoneMatch, NotModified: true}, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Result{}, fmt.Errorf("download failed: %s", res.Status)
	}
	tmp, err := os.CreateTemp(req.Dir, "mmdbwatch-*")
	if err != nil {
		return Result{}, err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, res.Body); err != nil {
		os.Remove(tmp.Name())
		return Result{}, err
	}
	if err := tmp.Sync(); err != nil {
		os.Remove(tmp.Name())
		return Result{}, err
	}
	path := tmp.Name()
	if strings.HasSuffix(strings.ToLower(req.URL), ".gz") {
		path, err = decompressGzip(tmp.Name())
		if err != nil {
			return Result{}, err
		}
	}
	return Result{Path: path, ETag: res.Header.Get("ETag")}, nil
}

func Text(ctx context.Context, url string, headers map[string]string, tlsConfig TLS) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	client, err := client(tlsConfig)
	if err != nil {
		return "", err
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("download failed: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	return string(b), err
}

func Bytes(ctx context.Context, url string, headers map[string]string, tlsConfig TLS) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	client, err := client(tlsConfig)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4*1024*1024))
}

func decompressGzip(path string) (string, error) {
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	out, err := os.CreateTemp(filepath.Dir(path), "mmdbwatch-*.mmdb")
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, gz); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	if err := out.Sync(); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	os.Remove(path)
	return out.Name(), nil
}

func client(cfg TLS) (*http.Client, error) {
	if cfg.ClientCert == "" && cfg.ClientKey == "" && cfg.CACert == "" && cfg.ServerName == "" {
		return &http.Client{Timeout: 5 * time.Minute}, nil
	}
	tlsConfig := &tls.Config{ServerName: cfg.ServerName}
	if cfg.ClientCert != "" || cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if cfg.CACert != "" {
		ca, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("failed to parse ca cert: %s", cfg.CACert)
		}
		tlsConfig.RootCAs = pool
	}
	return &http.Client{
		Timeout:   5 * time.Minute,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}
