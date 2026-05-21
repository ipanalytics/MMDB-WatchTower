package mmdbcheck

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/oschwald/maxminddb-golang"
	"mmdb-watchtower/internal/config"
)

type Info struct {
	Path         string `json:"path"`
	DatabaseType string `json:"database_type"`
	BuildEpoch   uint   `json:"build_epoch"`
	IPVersion    uint   `json:"ip_version"`
	RecordSize   uint   `json:"record_size"`
}

func Inspect(path string) (Info, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer db.Close()
	md := db.Metadata
	return Info{
		Path:         path,
		DatabaseType: md.DatabaseType,
		BuildEpoch:   md.BuildEpoch,
		IPVersion:    md.IPVersion,
		RecordSize:   md.RecordSize,
	}, nil
}

func Smoke(path string, required []string, tests []config.SmokeTest) error {
	db, err := maxminddb.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, test := range tests {
		ip := net.ParseIP(test.IP)
		if ip == nil {
			return fmt.Errorf("invalid smoke test ip: %s", test.IP)
		}
		var record map[string]any
		if err := db.Lookup(ip, &record); err != nil {
			return err
		}
		for _, field := range required {
			if _, ok := lookup(record, field); !ok {
				return fmt.Errorf("required field missing for %s: %s", test.IP, field)
			}
		}
		for field, expected := range test.Expect {
			actual, ok := lookup(record, field)
			if !ok {
				return fmt.Errorf("smoke test failed: %s missing", field)
			}
			if !equalScalar(actual, expected) {
				return fmt.Errorf("smoke test failed: %s expected %v got %v", field, expected, actual)
			}
		}
	}
	return nil
}

func lookup(record map[string]any, dotted string) (any, bool) {
	var cur any = record
	for _, part := range strings.Split(dotted, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func equalScalar(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
