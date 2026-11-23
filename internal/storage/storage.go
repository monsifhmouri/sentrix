// Package storage persists projects as checksummed JSON files.
//
// Writes are atomic (temp file + rename) so a crash never leaves a torn
// project file, and a mutex serializes access within the process. The
// checksum detects corruption and manual edits; Check, Backup and Restore all
// verify it. Cross-process locking is not implemented (see docs/limitations).
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"sentrix/pkg/models"
)

const (
	formatVersion = 1
	fileSuffix    = ".sentrix.json"
)

var ErrNotFound = errors.New("project not found")

type envelope struct {
	Version  int             `json:"format_version"`
	Checksum string          `json:"sha256"`
	SavedAt  time.Time       `json:"saved_at"`
	Project  json.RawMessage `json:"project"`
}

// Summary is the lightweight listing entry for a project.
type Summary struct {
	Name      string
	Domain    string
	Records   int
	UpdatedAt time.Time
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// Slug converts a project name into a safe file stem.
func Slug(name string) (string, error) {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if s == "" || len(s) > 64 {
		return "", fmt.Errorf("project name must produce 1-64 characters of [a-z0-9_-]")
	}
	return s, nil
}

func (s *Store) path(name string) (string, error) {
	slug, err := Slug(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, slug+fileSuffix), nil
}

func (s *Store) Exists(name string) bool {
	p, err := s.path(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func encode(p *models.Project) ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return json.Marshal(envelope{Version: formatVersion, Checksum: hex.EncodeToString(sum[:]), SavedAt: time.Now().UTC(), Project: raw})
}

func decode(data []byte) (*models.Project, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("corrupt project file: %w", err)
	}
	if env.Version != formatVersion {
		return nil, fmt.Errorf("unsupported project format version %d (this build reads version %d)", env.Version, formatVersion)
	}
	sum := sha256.Sum256(env.Project)
	if hex.EncodeToString(sum[:]) != env.Checksum {
		return nil, errors.New("integrity check failed: checksum mismatch (file modified or corrupted)")
	}
	var p models.Project
	if err := json.Unmarshal(env.Project, &p); err != nil {
		return nil, fmt.Errorf("corrupt project data: %w", err)
	}
	return &p, nil
}

func (s *Store) Save(p *models.Project) error {
	path, err := s.path(p.Name)
	if err != nil {
		return err
	}
	data, err := encode(p)
	if err != nil {
		return fmt.Errorf("encode project: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (s *Store) Load(name string) (*models.Project, error) {
	path, err := s.path(name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return nil, err
	}
	return decode(data)
}

// Check verifies the integrity of a stored project without keeping it.
func (s *Store) Check(name string) error {
	_, err := s.Load(name)
	return err
}

func (s *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileSuffix) {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), fileSuffix)
		p, err := s.Load(stem)
		if err != nil { // surface damaged projects instead of hiding them
			out = append(out, Summary{Name: stem + " (unreadable: " + err.Error() + ")"})
			continue
		}
		out = append(out, Summary{Name: p.Name, Domain: p.Domain, Records: len(p.Records), UpdatedAt: p.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Backup writes a verified copy of the project file to dest.
func (s *Store) Backup(name, dest string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return err
	}
	if _, err := decode(data); err != nil {
		return fmt.Errorf("refusing to back up a damaged project: %w", err)
	}
	return writeAtomic(dest, data)
}

// Restore loads a backup file into the store and returns the project name.
// It refuses to replace an existing project unless overwrite is set.
func (s *Store) Restore(src string, overwrite bool) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	p, err := decode(data)
	if err != nil {
		return "", err
	}
	path, err := s.path(p.Name)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(path); err == nil && !overwrite {
		return "", fmt.Errorf("project %q already exists (use overwrite to replace it)", p.Name)
	}
	return p.Name, writeAtomic(path, data)
}
