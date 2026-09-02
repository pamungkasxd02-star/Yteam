package snapshot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	manifestName = "manifest.json"
	stagingMark  = ".tmp"
)

var (
	ErrNotFound    = errors.New("snapshot not found")
	ErrUnsafePath  = errors.New("snapshot path escapes project root")
	ErrUnsupported = errors.New("unsupported filesystem entry")
)

// Entry is the portable representation of one project filesystem object.
// Paths are stored with slash separators so manifests are portable between
// Windows and Unix hosts.
type Entry struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Mode       uint32 `json:"mode,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	LinkTarget string `json:"link_target,omitempty"`
}

type Manifest struct {
	ID        string  `json:"id"`
	Root      string  `json:"root"`
	CreatedAt string  `json:"created_at"`
	Entries   []Entry `json:"entries"`
}

type Snapshot struct {
	Manifest Manifest
	Dir      string
}

// Service owns durable filesystem snapshots for a project. Snapshot data is
// kept below home, never inside the project, and all restore paths are checked
// against the project root before touching the filesystem.
type Service struct {
	root string
	home string
}

func New(home, root string) (*Service, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("snapshot project root is empty")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	absoluteHome, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	relativeHome, err := filepath.Rel(absoluteRoot, absoluteHome)
	if err != nil {
		return nil, err
	}
	if relativeHome == "." || (relativeHome != ".." && !strings.HasPrefix(relativeHome, ".."+string(filepath.Separator))) {
		return nil, errors.New("snapshot home must be outside project root")
	}
	if err := os.MkdirAll(filepath.Join(absoluteHome, "snapshots"), 0o700); err != nil {
		return nil, err
	}
	return &Service{root: absoluteRoot, home: filepath.Join(absoluteHome, "snapshots")}, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Capture() (*Snapshot, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.home, id)
	tmp := dir + stagingMark
	if err := os.RemoveAll(tmp); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(tmp, "files"), 0o700); err != nil {
		return nil, err
	}
	entries, err := s.collect()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	for _, entry := range entries {
		if entry.Kind != "file" {
			continue
		}
		source, err := s.safePath(entry.Path)
		if err != nil {
			_ = os.RemoveAll(tmp)
			return nil, err
		}
		destination := filepath.Join(tmp, "files", filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			_ = os.RemoveAll(tmp)
			return nil, err
		}
		if err := copyFile(source, destination, os.FileMode(entry.Mode)); err != nil {
			_ = os.RemoveAll(tmp)
			return nil, err
		}
	}
	manifest := Manifest{ID: id, Root: s.root, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Entries: entries}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, manifestName), append(data, '\n'), 0o600); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	return &Snapshot{Manifest: manifest, Dir: dir}, nil
}

func (s *Service) Load(id string) (*Snapshot, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	dir := filepath.Join(s.home, id)
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.ID != id || filepath.Clean(manifest.Root) != filepath.Clean(s.root) {
		return nil, errors.New("snapshot manifest does not belong to project")
	}
	return &Snapshot{Manifest: manifest, Dir: dir}, nil
}

func (s *Service) Remove(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	if err := os.RemoveAll(filepath.Join(s.home, id)); err != nil {
		return err
	}
	return nil
}

// Diff returns a compact, deterministic status between a saved snapshot and
// the current project. The actual bytes remain in the snapshot for restore.
func (s *Service) Diff(id string) (string, error) {
	saved, err := s.Load(id)
	if err != nil {
		return "", err
	}
	current, err := s.collect()
	if err != nil {
		return "", err
	}
	old := entryMap(saved.Manifest.Entries)
	now := entryMap(current)
	paths := make([]string, 0, len(old)+len(now))
	seen := map[string]bool{}
	for path := range old {
		seen[path] = true
	}
	for path := range now {
		seen[path] = true
	}
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var out strings.Builder
	for _, path := range paths {
		before, hadBefore := old[path]
		after, hadAfter := now[path]
		switch {
		case !hadBefore:
			fmt.Fprintf(&out, "A %s\n", path)
		case !hadAfter:
			fmt.Fprintf(&out, "D %s\n", path)
		case before.Kind != after.Kind || before.SHA256 != after.SHA256 || before.LinkTarget != after.LinkTarget || before.Mode != after.Mode:
			fmt.Fprintf(&out, "M %s\n", path)
		}
	}
	return out.String(), nil
}

// Restore applies a saved tree and removes project files which did not exist
// in that tree. A rollback snapshot is created first, so a failed restore
// attempts to leave the project as it was before the operation.
func (s *Service) Restore(id string) error {
	saved, err := s.Load(id)
	if err != nil {
		return err
	}
	if err := validateManifest(saved.Manifest); err != nil {
		return err
	}
	rollback, err := s.Capture()
	if err != nil {
		return err
	}
	if err := s.apply(saved); err != nil {
		_ = s.apply(rollback)
		_ = s.Remove(rollback.Manifest.ID)
		return err
	}
	_ = s.Remove(rollback.Manifest.ID)
	return nil
}

func (s *Service) apply(saved *Snapshot) error {
	keep := entryMap(saved.Manifest.Entries)
	current, err := s.collect()
	if err != nil {
		return err
	}
	for _, entry := range current {
		if _, ok := keep[entry.Path]; ok {
			continue
		}
		path, err := s.safePath(entry.Path)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	for _, entry := range saved.Manifest.Entries {
		path, err := s.safePath(entry.Path)
		if err != nil {
			return err
		}
		switch entry.Kind {
		case "dir":
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			if err := os.MkdirAll(path, os.FileMode(entry.Mode).Perm()); err != nil {
				return err
			}
		case "file":
			data, err := os.ReadFile(filepath.Join(saved.Dir, "files", filepath.FromSlash(entry.Path)))
			if err != nil {
				return err
			}
			if entry.Size != int64(len(data)) || entry.SHA256 == "" {
				return errors.New("snapshot file metadata mismatch")
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != entry.SHA256 {
				return errors.New("snapshot file checksum mismatch")
			}
			if err := replaceFile(path, data, os.FileMode(entry.Mode)); err != nil {
				return err
			}
		case "symlink":
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.Symlink(entry.LinkTarget, path); err != nil {
				return err
			}
		default:
			return ErrUnsupported
		}
	}
	return nil
}

func (s *Service) collect() ([]Entry, error) {
	entries := []Entry{}
	err := filepath.Walk(s.root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == s.root {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			entries = append(entries, Entry{Path: rel, Kind: "dir", Mode: uint32(info.Mode().Perm())})
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, Entry{Path: rel, Kind: "symlink", Mode: uint32(info.Mode()), LinkTarget: target})
			return nil
		}
		if !info.Mode().IsRegular() {
			return ErrUnsupported
		}
		hash, err := fileHash(path)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Path: rel, Kind: "file", Mode: uint32(info.Mode().Perm()), Size: info.Size(), SHA256: hash})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (s *Service) safePath(relative string) (string, error) {
	if relative == "" || strings.Contains(relative, `\`) || filepath.IsAbs(filepath.FromSlash(relative)) || filepath.VolumeName(filepath.FromSlash(relative)) != "" {
		return "", ErrUnsafePath
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", ErrUnsafePath
		}
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	path := filepath.Join(s.root, clean)
	if filepath.Clean(path) != filepath.Join(s.root, clean) {
		return "", ErrUnsafePath
	}
	parent := filepath.Dir(path)
	for parent != s.root && strings.HasPrefix(parent, s.root+string(filepath.Separator)) {
		info, err := os.Lstat(parent)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafePath
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent = filepath.Dir(parent)
	}
	return path, nil
}

func entryMap(entries []Entry) map[string]Entry {
	result := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		result[entry.Path] = entry
	}
	return result
}

func validateManifest(manifest Manifest) error {
	seen := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if entry.Path == "" || strings.Contains(entry.Path, `\`) || filepath.IsAbs(filepath.FromSlash(entry.Path)) || filepath.VolumeName(filepath.FromSlash(entry.Path)) != "" {
			return ErrUnsafePath
		}
		for _, part := range strings.Split(entry.Path, "/") {
			if part == "" || part == "." || part == ".." {
				return ErrUnsafePath
			}
		}
		if _, ok := seen[entry.Path]; ok {
			return errors.New("snapshot manifest contains duplicate path")
		}
		seen[entry.Path] = struct{}{}
		switch entry.Kind {
		case "dir":
		case "file":
			if entry.Size < 0 || entry.SHA256 == "" {
				return errors.New("snapshot file metadata is incomplete")
			}
		case "symlink":
			if entry.LinkTarget == "" {
				return errors.New("snapshot symlink target is empty")
			}
		default:
			return ErrUnsupported
		}
	}
	return nil
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func replaceFile(path string, data []byte, mode os.FileMode) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".yteam-snapshot-tmp"
	if err := os.WriteFile(tmp, data, mode.Perm()); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode.Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "snp_" + hex.EncodeToString(buf), nil
}

func validID(id string) bool {
	return strings.HasPrefix(id, "snp_") && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}
