package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	topologyTTL    = 7 * 24 * time.Hour
	updateCheckTTL = time.Hour
)

var (
	// ErrNotFound distinguishes an absent cache file from an unreadable one.
	ErrNotFound = errors.New("cache entry not found")
	// ErrUnsupportedSchema indicates a cache file written by an incompatible version.
	ErrUnsupportedSchema = errors.New("unsupported cache schema")
)

// Store provides access to the local cache for one Azure account namespace.
type Store struct {
	directory string
}

// NewStore creates a store rooted at the supplied account namespace.
func NewStore(namespace string) (*Store, error) {
	directory, err := AccountDirectory(namespace)
	if err != nil {
		return nil, err
	}
	return &Store{directory: directory}, nil
}

// AccountDirectory returns the account-specific directory below the user's
// cache directory. Namespace may contain relative path components, but cannot
// escape the azssh cache root.
func AccountDirectory(namespace string) (string, error) {
	if namespace == "" || filepath.IsAbs(namespace) {
		return "", fmt.Errorf("invalid cache namespace %q", namespace)
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	root := filepath.Join(cacheDir, "azssh")
	directory := filepath.Join(root, namespace)
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid cache namespace %q", namespace)
	}
	return directory, nil
}

// LoadTopology reads the cached topology snapshot.
func (s *Store) LoadTopology() (TopologySnapshot, error) {
	var envelope topologyEnvelope
	if err := s.load("topology.json", &envelope); err != nil {
		return TopologySnapshot{}, err
	}
	if envelope.SchemaVersion != schemaVersion {
		return TopologySnapshot{}, fmt.Errorf("topology: %w: %d", ErrUnsupportedSchema, envelope.SchemaVersion)
	}
	return envelope.Data, nil
}

// SaveTopology atomically saves a topology snapshot.
func (s *Store) SaveTopology(snapshot TopologySnapshot) error {
	return s.save("topology.json", topologyEnvelope{SchemaVersion: schemaVersion, Data: snapshot})
}

// LoadVMInventory reads the cached eligible VM inventory snapshot.
func (s *Store) LoadVMInventory() (VMInventorySnapshot, error) {
	var envelope vmInventoryEnvelope
	if err := s.load("vm-inventory.json", &envelope); err != nil {
		return VMInventorySnapshot{}, err
	}
	if envelope.SchemaVersion != schemaVersion {
		return VMInventorySnapshot{}, fmt.Errorf("VM inventory: %w: %d", ErrUnsupportedSchema, envelope.SchemaVersion)
	}
	return envelope.Data, nil
}

// SaveVMInventory atomically saves an eligible VM inventory snapshot.
func (s *Store) SaveVMInventory(snapshot VMInventorySnapshot) error {
	return s.save("vm-inventory.json", vmInventoryEnvelope{SchemaVersion: schemaVersion, Data: snapshot})
}

// LoadUpdateCheck reads the most recent release lookup.
func (s *Store) LoadUpdateCheck() (UpdateCheck, error) {
	var envelope updateCheckEnvelope
	if err := s.load("update-check.json", &envelope); err != nil {
		return UpdateCheck{}, err
	}
	if envelope.SchemaVersion != schemaVersion {
		return UpdateCheck{}, fmt.Errorf("update check: %w: %d", ErrUnsupportedSchema, envelope.SchemaVersion)
	}
	return envelope.Data, nil
}

// SaveUpdateCheck atomically saves the most recent release lookup.
func (s *Store) SaveUpdateCheck(check UpdateCheck) error {
	return s.save("update-check.json", updateCheckEnvelope{SchemaVersion: schemaVersion, Data: check})
}

// IsTopologyFresh reports whether snapshot was fetched within the topology TTL.
func IsTopologyFresh(snapshot TopologySnapshot, now time.Time) bool {
	return !snapshot.FetchedAt.IsZero() && !snapshot.FetchedAt.After(now) && now.Sub(snapshot.FetchedAt) <= topologyTTL
}

// IsUpdateCheckFresh reports whether a release lookup was attempted within an hour.
func IsUpdateCheckFresh(check UpdateCheck, now time.Time) bool {
	return !check.CheckedAt.IsZero() && !check.CheckedAt.After(now) && now.Sub(check.CheckedAt) <= updateCheckTTL
}

func (s *Store) load(name string, destination any) error {
	contents, err := os.ReadFile(filepath.Join(s.directory, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", name, ErrNotFound)
		}
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(contents, destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func (s *Store) save(name string, value any) error {
	contents, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	temporary, err := os.CreateTemp(s.directory, "."+name+"-*")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", name, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary %s: %w", name, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary %s: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", name, err)
	}
	if err := os.Rename(temporaryName, filepath.Join(s.directory, name)); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
