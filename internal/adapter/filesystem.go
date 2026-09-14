package adapter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/operation"
	"github.com/AllenMuu/skill-manager/internal/resource"
)

var (
	// ErrForceRequired is returned before confirmation when replacement would
	// remove an unmanaged destination.
	ErrForceRequired = errors.New("conflict strategy requires force confirmation")
	// ErrUnsafePath protects source and destination ownership boundaries.
	ErrUnsafePath = errors.New("refusing unmanaged or unexpected path")
	// ErrNotConfirmed indicates that no mutation was authorized.
	ErrNotConfirmed = operation.ErrNotConfirmed
	errLateConflict = errors.New("destination changed during guarded replacement")
)

// ConflictStrategy controls an existing destination. The zero value refuses
// all conflicts; replacement is only available with Force and confirmation.
type ConflictStrategy string

const ConflictReplace ConflictStrategy = "replace"

// FilesystemPlacementOptions supplies the explicit, guarded mutation inputs.
// Resource content is treated as opaque bytes and is never interpreted.
type FilesystemPlacementOptions struct {
	Destination string
	// SourceContent optionally materializes a managed regular-file source after
	// confirmation. It is used by rendered SubAgents; nil preserves the
	// directory-backed Skill behavior.
	SourceContent []byte
	Conflict      ConflictStrategy
	Force         bool
	Journal       *operation.Journal
	Confirm       func(operation.Plan) bool
	// BeforePublish is a test and integration seam invoked after the final
	// confirmation re-check and before publication.
	BeforePublish func() error
	// BeforeRemove is invoked after an existing destination is atomically
	// staged and immediately before its staged copy would be discarded.
	BeforeRemove func() error
	// BeforeFinalPublish is invoked after the final destination absence check.
	BeforeFinalPublish func() error
}

// PlaceFilesystem publishes a source directory as an absolute symlink at the
// requested destination. It previews and confirms before changing anything,
// journals the old state for undo, and restores it if publication fails.
func PlaceFilesystem(plan resource.PlacementPlan, options FilesystemPlacementOptions) (operation.Plan, error) {
	if err := plan.Resource.Validate(); err != nil {
		return operation.Plan{}, err
	}
	if options.Journal == nil {
		return operation.Plan{}, fmt.Errorf("filesystem placement requires an operation journal")
	}
	source, destination, err := safePlacementPaths(plan.Resource, options.Destination)
	if err != nil {
		return operation.Plan{}, err
	}
	if info, err := os.Lstat(source); err != nil {
		if !os.IsNotExist(err) || options.SourceContent == nil {
			return operation.Plan{}, fmt.Errorf("inspect resource source: %w", err)
		}
	} else if plan.Resource.Kind == resource.SubAgent && info.Mode().IsRegular() {
		if options.SourceContent == nil {
			return operation.Plan{}, fmt.Errorf("%w: resource source must be a directory or managed SubAgent file", ErrUnsafePath)
		}
		existing, readErr := os.ReadFile(source)
		if readErr != nil || !bytes.Equal(existing, options.SourceContent) {
			return operation.Plan{}, fmt.Errorf("%w: refusing to replace an existing rendered SubAgent source", ErrUnsafePath)
		}
	} else if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		// Rendered SubAgents are regular files; Skills remain directories.
	} else {
		return operation.Plan{}, fmt.Errorf("%w: resource source must be a directory or managed SubAgent file", ErrUnsafePath)
	}
	conflict, err := destinationConflict(destination, source)
	if err != nil {
		return operation.Plan{}, err
	}
	preview := operation.NewPlan("place resource")
	preview.ResourceKind = string(plan.Resource.Kind)
	if options.SourceContent != nil {
		preview.Changes = append(preview.Changes, operation.Change{Path: source, Action: "write managed SubAgent source"})
	}
	preview.Changes = append(preview.Changes, operation.Change{Path: destination, Action: "create absolute link", Detail: source})
	if conflict {
		if options.Conflict != ConflictReplace {
			return preview, ErrUnsafePath
		}
		if !options.Force {
			return preview, ErrForceRequired
		}
		preview.Changes[len(preview.Changes)-1].Action = "replace conflicting path with absolute link"
	}
	if options.Confirm == nil || !options.Confirm(preview) {
		return preview, ErrNotConfirmed
	}

	// Re-check the source and destination after confirmation to close the
	// concurrent-edit window. Capture happens only after confirmation so a
	// declined preview leaves no journal backup behind.
	if options.SourceContent == nil {
		if err := verifySource(source, plan.Resource.Kind == resource.SubAgent); err != nil {
			return preview, err
		}
	}
	conflict, err = destinationConflict(destination, source)
	if err != nil {
		return preview, err
	}
	if conflict && (options.Conflict != ConflictReplace || !options.Force) {
		return preview, ErrUnsafePath
	}
	paths := []string{destination}
	if options.SourceContent != nil {
		paths = append([]string{source}, paths...)
	}
	before, err := options.Journal.Capture(paths)
	if err != nil {
		return preview, err
	}
	if options.SourceContent != nil {
		if err := writeManagedSource(source, options.SourceContent); err != nil {
			return preview, errors.Join(err, options.Journal.Restore(before))
		}
		if err := verifySource(source, true); err != nil {
			return preview, errors.Join(err, options.Journal.Restore(before))
		}
	}
	if options.BeforePublish != nil {
		if err := options.BeforePublish(); err != nil {
			if options.SourceContent != nil {
				return preview, errors.Join(err, options.Journal.Restore(before))
			}
			return preview, err
		}
	}
	conflictSnapshot := before[len(before)-1]
	if conflict && !matchesSnapshot(conflictSnapshot) {
		return preview, ErrUnsafePath
	}
	if err := publishLink(destination, source, conflict, conflictSnapshot, options.BeforeRemove, options.BeforeFinalPublish); err != nil {
		// A rejected late conflict has not mutated the destination; restoring an
		// empty snapshot here would wrongly delete the unmanaged file that won
		// the race.
		if options.SourceContent != nil || (conflict && !errors.Is(err, errLateConflict)) {
			return preview, errors.Join(err, options.Journal.Restore(before))
		}
		return preview, err
	}
	after, err := options.Journal.Capture(paths)
	if err != nil {
		return preview, errors.Join(err, options.Journal.Restore(before))
	}
	if err := options.Journal.RecordPlan(preview, before, after); err != nil {
		return preview, errors.Join(err, options.Journal.Restore(before))
	}
	return preview, nil
}

func matchesSnapshot(snapshot operation.Snapshot) bool {
	current, err := os.Lstat(snapshot.Path)
	if err != nil {
		return false
	}
	backup, err := os.Lstat(snapshot.Backup)
	if err != nil || current.Mode() != backup.Mode() {
		return false
	}
	if current.Mode()&os.ModeSymlink != 0 {
		currentTarget, currentErr := os.Readlink(snapshot.Path)
		backupTarget, backupErr := os.Readlink(snapshot.Backup)
		return currentErr == nil && backupErr == nil && currentTarget == backupTarget
	}
	if current.Mode().IsRegular() {
		currentBytes, currentErr := os.ReadFile(snapshot.Path)
		backupBytes, backupErr := os.ReadFile(snapshot.Backup)
		return currentErr == nil && backupErr == nil && string(currentBytes) == string(backupBytes)
	}
	if current.IsDir() {
		return treesMatch(snapshot.Path, snapshot.Backup)
	}
	return true
}

// treesMatch compares directory contents without following symlinks. Journal
// backups are therefore treated as opaque filesystem snapshots, including
// executable files and nested unmanaged content.
func treesMatch(current, backup string) bool {
	currentEntries, err := os.ReadDir(current)
	if err != nil {
		return false
	}
	backupEntries, err := os.ReadDir(backup)
	if err != nil || len(currentEntries) != len(backupEntries) {
		return false
	}
	for _, entry := range currentEntries {
		other, err := os.ReadDir(backup)
		if err != nil {
			return false
		}
		found := false
		for _, candidate := range other {
			if candidate.Name() == entry.Name() {
				found = true
				break
			}
		}
		if !found || !pathsMatch(filepath.Join(current, entry.Name()), filepath.Join(backup, entry.Name())) {
			return false
		}
	}
	return true
}

func pathsMatch(current, backup string) bool {
	currentInfo, err := os.Lstat(current)
	if err != nil {
		return false
	}
	backupInfo, err := os.Lstat(backup)
	if err != nil || currentInfo.Mode() != backupInfo.Mode() {
		return false
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 {
		currentTarget, currentErr := os.Readlink(current)
		backupTarget, backupErr := os.Readlink(backup)
		return currentErr == nil && backupErr == nil && currentTarget == backupTarget
	}
	if currentInfo.IsDir() {
		return treesMatch(current, backup)
	}
	if currentInfo.Mode().IsRegular() {
		currentBytes, currentErr := os.ReadFile(current)
		backupBytes, backupErr := os.ReadFile(backup)
		return currentErr == nil && backupErr == nil && bytes.Equal(currentBytes, backupBytes)
	}
	return true
}

func safePlacementPaths(managed resource.ManagedResource, destination string) (string, string, error) {
	if managed.Provenance.Source == "" || destination == "" {
		return "", "", fmt.Errorf("%w: source and destination are required", ErrUnsafePath)
	}
	source, err := filepath.Abs(managed.Provenance.Source)
	if err != nil {
		return "", "", fmt.Errorf("%w: source and destination are required", ErrUnsafePath)
	}
	destination, err = filepath.Abs(destination)
	if err != nil || source == destination {
		return "", "", fmt.Errorf("%w: source and destination must differ", ErrUnsafePath)
	}
	base := filepath.Base(destination)
	if managed.Kind == resource.SubAgent {
		if strings.TrimSuffix(base, filepath.Ext(base)) != managed.ID {
			return "", "", fmt.Errorf("%w: destination basename does not match resource identifier", ErrUnsafePath)
		}
	} else if base != managed.ID {
		return "", "", fmt.Errorf("%w: destination basename does not match resource identifier", ErrUnsafePath)
	}
	return source, destination, nil
}

func verifySource(source string, allowFile bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect resource source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !(allowFile && info.Mode().IsRegular())) {
		return fmt.Errorf("%w: resource source must remain a directory or managed file", ErrUnsafePath)
	}
	return nil
}

func writeManagedSource(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-manager-render-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func destinationConflict(destination, source string) (bool, error) {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(destination)
		if readErr == nil && target == source {
			return false, nil
		}
	}
	return true, nil
}

func publishLink(destination, source string, replace bool, snapshot operation.Snapshot, beforeRemove, beforeFinalPublish func() error) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if replace {
		stageRoot, err := os.MkdirTemp(filepath.Dir(destination), ".skill-manager-replace-")
		if err != nil {
			return err
		}
		stage := filepath.Join(stageRoot, "existing")
		if err := os.Rename(destination, stage); err != nil {
			os.RemoveAll(stageRoot)
			return err
		}
		if beforeRemove != nil {
			if err := beforeRemove(); err != nil {
				return err
			}
		}
		if _, err := os.Lstat(destination); err == nil {
			// A new owner appeared after staging. Leave it untouched and retain
			// the staged prior content for recovery rather than overwriting it.
			return errors.Join(ErrUnsafePath, errLateConflict)
		} else if !os.IsNotExist(err) {
			return err
		}
		if !pathsMatch(stage, snapshot.Backup) {
			return errors.Join(ErrUnsafePath, errLateConflict)
		}
		if err := os.RemoveAll(stageRoot); err != nil {
			return err
		}
	} else if target, err := os.Readlink(destination); err == nil && target == source {
		return nil
	} else if _, err := os.Lstat(destination); err == nil {
		return ErrUnsafePath
	} else if !os.IsNotExist(err) {
		return err
	}
	if beforeFinalPublish != nil {
		if err := beforeFinalPublish(); err != nil {
			return err
		}
	}
	// Symlink itself is an atomic create and refuses an existing destination,
	// unlike Rename which would overwrite a late unmanaged path.
	if err := os.Symlink(source, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.Join(ErrUnsafePath, errLateConflict)
		}
		return err
	}
	return nil
}
