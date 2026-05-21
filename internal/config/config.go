package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Databases []Database `yaml:"databases" json:"databases"`
	Reload    Reload     `yaml:"reload" json:"reload"`
	Retention Retention  `yaml:"retention" json:"retention"`
	Metrics   Metrics    `yaml:"metrics" json:"metrics"`
	Readiness Readiness  `yaml:"readiness" json:"readiness"`
}

type Database struct {
	Name           string            `yaml:"name" json:"name"`
	URL            string            `yaml:"url" json:"url"`
	Path           string            `yaml:"path" json:"path"`
	SHA256URL      string            `yaml:"sha256_url" json:"sha256_url"`
	SignatureURL   string            `yaml:"signature_url" json:"signature_url"`
	CosignKey      string            `yaml:"cosign_key" json:"cosign_key"`
	Channel        string            `yaml:"channel" json:"channel"`
	Channels       map[string]Source `yaml:"channels" json:"channels"`
	Interval       Duration          `yaml:"interval" json:"interval"`
	MinSizeMB      int64             `yaml:"min_size_mb" json:"min_size_mb"`
	MaxSizeMB      int64             `yaml:"max_size_mb" json:"max_size_mb"`
	RequiredFields []string          `yaml:"required_fields" json:"required_fields"`
	SmokeTests     []SmokeTest       `yaml:"smoke_tests" json:"smoke_tests"`
	Headers        map[string]string `yaml:"headers" json:"headers"`
	TLS            TLS               `yaml:"tls" json:"tls"`
}

type Source struct {
	URL          string `yaml:"url" json:"url"`
	SHA256URL    string `yaml:"sha256_url" json:"sha256_url"`
	SignatureURL string `yaml:"signature_url" json:"signature_url"`
	CosignKey    string `yaml:"cosign_key" json:"cosign_key"`
}

type TLS struct {
	ClientCert string `yaml:"client_cert" json:"client_cert"`
	ClientKey  string `yaml:"client_key" json:"client_key"`
	CACert     string `yaml:"ca_cert" json:"ca_cert"`
	ServerName string `yaml:"server_name" json:"server_name"`
}

type SmokeTest struct {
	IP     string         `yaml:"ip" json:"ip"`
	Expect map[string]any `yaml:"expect" json:"expect"`
}

type Reload struct {
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args" json:"args"`
	Timeout Duration `yaml:"timeout" json:"timeout"`
}

type Retention struct {
	KeepVersions int `yaml:"keep_versions" json:"keep_versions"`
}

type Metrics struct {
	Listen string `yaml:"listen" json:"listen"`
}

type Readiness struct {
	Listen string `yaml:"listen" json:"listen"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", d.Duration.String())), nil
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Retention.KeepVersions == 0 {
		cfg.Retention.KeepVersions = 5
	}
	for i, db := range cfg.Databases {
		if db.Name == "" {
			return Config{}, fmt.Errorf("databases[%d].name is required", i)
		}
		if db.URL == "" && len(db.Channels) == 0 {
			return Config{}, fmt.Errorf("databases[%s].url or channels is required", db.Name)
		}
		if db.Path == "" {
			return Config{}, fmt.Errorf("databases[%s].path is required", db.Name)
		}
	}
	return cfg, nil
}

func ResolveDatabase(db Database, channel string) (Database, error) {
	if channel == "" {
		channel = db.Channel
	}
	if channel == "" {
		channel = "stable"
	}
	if len(db.Channels) == 0 {
		return db, nil
	}
	src, ok := db.Channels[channel]
	if !ok {
		return Database{}, fmt.Errorf("database %s has no channel %q", db.Name, channel)
	}
	db.Channel = channel
	db.URL = src.URL
	db.SHA256URL = src.SHA256URL
	db.SignatureURL = src.SignatureURL
	db.CosignKey = src.CosignKey
	return db, nil
}

const ExampleYAML = `databases:
  - name: geo
    channel: stable
    channels:
      stable:
        url: https://example.com/mmdb/geo/stable.mmdb.gz
        sha256_url: https://example.com/mmdb/geo/stable.sha256
      canary:
        url: https://example.com/mmdb/geo/canary.mmdb.gz
      latest:
        url: https://example.com/mmdb/geo/latest.mmdb.gz
    path: /var/lib/mmdb/geo.mmdb
    signature_url: https://example.com/mmdb/geo/stable.mmdb.sig
    cosign_key: /etc/mmdbwatch/cosign.pub
    interval: 6h
    min_size_mb: 20
    max_size_mb: 500
    tls:
      client_cert: /etc/mmdbwatch/client.crt
      client_key: /etc/mmdbwatch/client.key
      ca_cert: /etc/mmdbwatch/ca.crt
    required_fields:
      - location.country_code
      - location.city_geoname_id
      - confidence
    smoke_tests:
      - ip: 8.8.8.8
        expect:
          location.country_code: US

  - name: vpn
    url: https://example.com/mmdb/vpn/latest.mmdb
    path: /var/lib/mmdb/vpn.mmdb
    interval: 1h
    min_size_mb: 5
    required_fields:
      - privacy.is_vpn
      - confidence
    smoke_tests:
      - ip: 91.196.220.30
        expect:
          privacy.is_vpn: true

reload:
  command: systemctl reload my-ip-api
  timeout: 10s

retention:
  keep_versions: 5

metrics:
  listen: 127.0.0.1:9417

readiness:
  listen: 127.0.0.1:9418
`
