package operation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrNotConfirmed     = fmt.Errorf("operation was not confirmed")
	ErrUnexpectedState  = fmt.Errorf("operation paths changed since confirmation")
	ErrJournalCommitted = fmt.Errorf("operation journal was published but post-commit sync failed")
)

// Snapshot records a path's state in an operation backup directory.
type Snapshot struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Backup string `json:"backup,omitempty"`
}

// Entry is a confirmed reversible operation.
type Entry struct {
	Version      string     `json:"version,omitempty"`
	ResourceKind string     `json:"resourceKind,omitempty"`
	Operation    string     `json:"operation"`
	At           time.Time  `json:"at"`
	Before       []Snapshot `json:"before"`
	After        []Snapshot `json:"after"`
}

// Journal persists reversible operation entries at Path.
type Journal struct {
	Path string
	// BeforeRestorePublish is an optional fault-injection seam for restore tests.
	BeforeRestorePublish func(path string) error
}

// New creates a journal that persists entries at path.
func New(path string) *Journal { return &Journal{Path: path} }

// Capture snapshots each explicit path without following soft links.
func (j *Journal) Capture(paths []string) ([]Snapshot, error) {
	result := make([]Snapshot, 0, len(paths))
	for i, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve snapshot path: %w", err)
		}
		s := Snapshot{Path: abs}
		if _, err := os.Lstat(abs); err != nil {
			if os.IsNotExist(err) {
				result = append(result, s)
				continue
			}
			return nil, fmt.Errorf("inspect %s: %w", abs, err)
		}
		s.Exists = true
		backup := filepath.Join(filepath.Dir(j.Path), ".skill-manager-journal", fmt.Sprintf("%d-%d", time.Now().UnixNano(), i))
		if err := copyPath(abs, backup); err != nil {
			return nil, err
		}
		s.Backup = backup
		result = append(result, s)
	}
	return result, nil
}

// Record appends a confirmed operation with both pre- and post-operation state.
func (j *Journal) Record(operation string, before, after []Snapshot) error {
	entries, err := j.entries()
	if err != nil {
		return err
	}
	entries = append(entries, Entry{Version: "v1", ResourceKind: "skill", Operation: operation, At: time.Now().UTC(), Before: before, After: after})
	return j.write(entries)
}

// Latest returns the latest journal entry after normalizing legacy fields in
// memory. It never rewrites an existing journal merely by reading it.
func (j *Journal) Latest() (Entry, bool, error) {
	entries, err := j.entries()
	if err != nil {
		return Entry{}, false, err
	}
	if len(entries) == 0 {
		return Entry{}, false, nil
	}
	return entries[len(entries)-1], true, nil
}

// RecordedLinkTarget reports whether a confirmed operation recorded path as a
// soft link, returning its recorded destination without following it.
func (j *Journal) RecordedLinkTarget(path string) (string, bool, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}
	entries, err := j.entries()
	if err != nil {
		return "", false, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		for _, snapshot := range entries[i].After {
			if snapshot.Path != path {
				continue
			}
			// A path's newest post-operation state owns the answer. Never fall
			// through to an older managed-link snapshot after a fork/remove.
			if !snapshot.Exists {
				return "", false, nil
			}
			info, err := os.Lstat(snapshot.Backup)
			if err != nil {
				return "", false, fmt.Errorf("inspect journal backup: %w", err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return "", false, nil
			}
			target, err := os.Readlink(snapshot.Backup)
			if err != nil {
				return "", false, err
			}
			return target, true, nil
		}
	}
	return "", false, nil
}

// UndoLatest restores the pre-operation state of the latest journal entry.
func (j *Journal) UndoLatest(confirm func(Plan) bool) error {
	entries, err := j.entries()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("operation journal is empty")
	}
	entry := entries[len(entries)-1]
	plan := Plan{Operation: "undo " + entry.Operation}
	for _, snapshot := range entry.Before {
		plan.Changes = append(plan.Changes, Change{Path: snapshot.Path, Action: "restore pre-operation state"})
	}
	if confirm == nil || !confirm(plan) {
		return ErrNotConfirmed
	}
	for _, snapshot := range entry.After {
		matches, err := matchesSnapshot(snapshot)
		if err != nil {
			return err
		}
		if !matches {
			return ErrUnexpectedState
		}
	}
	if err := j.Restore(entry.Before); err != nil {
		return err
	}
	if err := j.write(entries[:len(entries)-1]); err != nil {
		if errors.Is(err, ErrJournalCommitted) {
			return err
		}
		return errors.Join(err, j.Restore(entry.After))
	}
	return nil
}

// Restore replaces the supplied explicit paths with their captured state.
// It is used to roll back an incomplete staged operation.
func (j *Journal) Restore(snapshots []Snapshot) error {
	// Validate every backup before touching any current path.
	for _, snapshot := range snapshots {
		if !snapshot.Exists {
			continue
		}
		if _, err := os.Lstat(snapshot.Backup); err != nil {
			return fmt.Errorf("preflight backup %s: %w", snapshot.Backup, err)
		}
	}
	// Build every replacement before removing an existing target.
	type stage struct{ root, candidate, previous string }
	staged := make([]stage, len(snapshots))
	for i, snapshot := range snapshots {
		root, err := os.MkdirTemp(filepath.Dir(snapshot.Path), ".skill-manager-restore-")
		if err != nil {
			return fmt.Errorf("stage restore %s: %w", snapshot.Path, err)
		}
		staged[i] = stage{root: root, candidate: filepath.Join(root, "candidate"), previous: filepath.Join(root, "previous")}
		defer os.RemoveAll(root)
		if !snapshot.Exists {
			continue
		}
		if err := copyPath(snapshot.Backup, staged[i].candidate); err != nil {
			return fmt.Errorf("stage restore %s: %w", snapshot.Path, err)
		}
	}
	moved := make([]bool, len(snapshots))
	published := make([]bool, len(snapshots))
	rollback := func() error {
		var errs []error
		for i := len(snapshots) - 1; i >= 0; i-- {
			if !moved[i] && !published[i] {
				continue
			}
			if published[i] {
				if err := os.RemoveAll(snapshots[i].Path); err != nil {
					errs = append(errs, err)
				}
			}
			if _, err := os.Lstat(staged[i].previous); err == nil {
				if err := os.Rename(staged[i].previous, snapshots[i].Path); err != nil {
					errs = append(errs, err)
				}
			}
		}
		return errors.Join(errs...)
	}
	for i, snapshot := range snapshots {
		if _, err := os.Lstat(snapshot.Path); err == nil {
			if err := os.Rename(snapshot.Path, staged[i].previous); err != nil {
				return errors.Join(fmt.Errorf("stage current %s: %w", snapshot.Path, err), rollback())
			}
			moved[i] = true
		} else if !os.IsNotExist(err) {
			return errors.Join(err, rollback())
		}
		if j.BeforeRestorePublish != nil {
			if err := j.BeforeRestorePublish(snapshot.Path); err != nil {
				return errors.Join(err, rollback())
			}
		}
		if snapshot.Exists {
			if err := os.Rename(staged[i].candidate, snapshot.Path); err != nil {
				return errors.Join(fmt.Errorf("publish restore %s: %w", snapshot.Path, err), rollback())
			}
		}
		published[i] = true
	}
	return nil
}

func (j *Journal) entries() ([]Entry, error) {
	b, err := os.ReadFile(j.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read operation journal: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("parse operation journal: %w", err)
	}
	for i := range entries {
		if entries[i].Version == "" {
			entries[i].Version = "v1"
		}
		if entries[i].ResourceKind == "" {
			entries[i].ResourceKind = "skill"
		}
		if entries[i].Version != "v1" {
			return nil, fmt.Errorf("unsupported operation journal version %q", entries[i].Version)
		}
	}
	return entries, nil
}

func (j *Journal) write(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o755); err != nil {
		return fmt.Errorf("create operation journal directory: %w", err)
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(j.Path), ".skill-manager-journal-")
	if err != nil {
		return fmt.Errorf("create journal temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write journal temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync journal temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close journal temp file: %w", err)
	}
	if err := os.Rename(tmpName, j.Path); err != nil {
		return fmt.Errorf("publish operation journal: %w", err)
	}
	if dir, err := os.Open(filepath.Dir(j.Path)); err == nil {
		defer dir.Close()
		if err := dir.Sync(); err != nil {
			return errors.Join(ErrJournalCommitted, fmt.Errorf("sync operation journal directory: %w", err))
		}
	}
	return nil
}

func copyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.Symlink(target, destination)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported snapshot file type at %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func matchesSnapshot(snapshot Snapshot) (bool, error) {
	_, err := os.Lstat(snapshot.Path)
	if !snapshot.Exists {
		if err == nil {
			return false, nil
		}
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("inspect expected absent path %s: %w", snapshot.Path, err)
	}
	if err != nil {
		return false, fmt.Errorf("inspect expected path %s: %w", snapshot.Path, err)
	}
	return pathsEqual(snapshot.Path, snapshot.Backup)
}

func pathsEqual(left, right string) (bool, error) {
	li, err := os.Lstat(left)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", left, err)
	}
	ri, err := os.Lstat(right)
	if err != nil || li.Mode() != ri.Mode() {
		if err != nil {
			return false, fmt.Errorf("inspect backup %s: %w", right, err)
		}
		return false, nil
	}
	if li.Mode()&os.ModeSymlink != 0 {
		l, le := os.Readlink(left)
		r, re := os.Readlink(right)
		if le != nil {
			return false, fmt.Errorf("read link %s: %w", left, le)
		}
		if re != nil {
			return false, fmt.Errorf("read backup link %s: %w", right, re)
		}
		return l == r, nil
	}
	if li.IsDir() {
		le, leErr := os.ReadDir(left)
		re, reErr := os.ReadDir(right)
		if leErr != nil || reErr != nil || len(le) != len(re) {
			if leErr != nil {
				return false, fmt.Errorf("read directory %s: %w", left, leErr)
			}
			if reErr != nil {
				return false, fmt.Errorf("read backup directory %s: %w", right, reErr)
			}
			return false, nil
		}
		for i := range le {
			if le[i].Name() != re[i].Name() {
				return false, nil
			}
			equal, err := pathsEqual(filepath.Join(left, le[i].Name()), filepath.Join(right, re[i].Name()))
			if err != nil {
				return false, err
			}
			if !equal {
				return false, nil
			}
		}
		return true, nil
	}
	if !li.Mode().IsRegular() {
		return false, nil
	}
	l, le := os.ReadFile(left)
	r, re := os.ReadFile(right)
	if le != nil {
		return false, fmt.Errorf("read %s: %w", left, le)
	}
	if re != nil {
		return false, fmt.Errorf("read backup %s: %w", right, re)
	}
	return bytes.Equal(l, r), nil
}
