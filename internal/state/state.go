package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"mmdb-watchtower/internal/config"
)

type Store struct {
	Path     string                    `json:"-"`
	Database map[string]DatabaseState  `json:"databases"`
	Backups  map[string][]VersionEntry `json:"backups"`
}

type DatabaseState struct {
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	Channel          string    `json:"channel,omitempty"`
	Status           string    `json:"status"`
	Version          string    `json:"version,omitempty"`
	DatabaseType     string    `json:"database_type,omitempty"`
	BuildEpoch       uint      `json:"build_epoch,omitempty"`
	SHA256           string    `json:"sha256,omitempty"`
	ETag             string    `json:"etag,omitempty"`
	LastUpdateAt     time.Time `json:"last_update_at,omitempty"`
	LastSuccessAt    time.Time `json:"last_success_at,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
	PreviousVersions int       `json:"previous_versions,omitempty"`
}

type VersionEntry struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256,omitempty"`
	BuildEpoch uint      `json:"build_epoch,omitempty"`
	Version    string    `json:"version,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func LoadForDB(db config.Database) (Store, error) {
	path := StatePath(db.Path)
	st := Store{Path: path, Database: map[string]DatabaseState{}, Backups: map[string][]VersionEntry{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	st.Path = path
	if st.Database == nil {
		st.Database = map[string]DatabaseState{}
	}
	if st.Backups == nil {
		st.Backups = map[string][]VersionEntry{}
	}
	return st, nil
}

func (s Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s Store) DatabaseState(name string) DatabaseState {
	return s.Database[name]
}

func (s Store) Versions(name string) []VersionEntry {
	versions := append([]VersionEntry(nil), s.Backups[name]...)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt.After(versions[j].CreatedAt)
	})
	return versions
}

func (s *Store) PutDatabase(db DatabaseState) {
	s.Database[db.Name] = db
}

func (s *Store) AddVersion(name string, version VersionEntry, keep int) {
	s.Backups[name] = append([]VersionEntry{version}, s.Backups[name]...)
	if keep > 0 && len(s.Backups[name]) > keep {
		for _, old := range s.Backups[name][keep:] {
			os.Remove(old.Path)
		}
		s.Backups[name] = s.Backups[name][:keep]
	}
}

func StatePath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), ".mmdbwatch", "state.json")
}

func VersionDir(dbPath, name string) string {
	return filepath.Join(filepath.Dir(dbPath), ".mmdbwatch", "versions", name)
}
