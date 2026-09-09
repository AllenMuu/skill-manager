package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
)

// ConflictStrategy controls an existing destination. The zero value refuses
// all conflicts; replacement is only available with Force and confirmation.
type ConflictStrategy string

const ConflictReplace ConflictStrategy = "replace"

// FilesystemPlacementOptions supplies the explicit, guarded mutation inputs.
// Resource content is treated as opaque bytes and is never interpreted.
type FilesystemPlacementOptions struct {
	Destination string
	Conflict    ConflictStrategy
	Force       bool
	Journal     *operation.Journal
	Confirm     func(operation.Plan) bool
	// BeforePublish is a test and integration seam invoked after the final
	// confirmation re-check and before publication.
	BeforePublish func() error
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
		return operation.Plan{}, fmt.Errorf("inspect resource source: %w", err)
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return operation.Plan{}, fmt.Errorf("%w: resource source must be a directory", ErrUnsafePath)
	}
	conflict, err := destinationConflict(destination, source)
	if err != nil {
		return operation.Plan{}, err
	}
	preview := operation.NewPlan("place resource")
	preview.ResourceKind = string(plan.Resource.Kind)
	preview.Changes = append(preview.Changes, operation.Change{Path: destination, Action: "create absolute link", Detail: source})
	if conflict {
		if options.Conflict != ConflictReplace {
			return preview, ErrUnsafePath
		}
		if !options.Force {
			return preview, ErrForceRequired
		}
		preview.Changes[0].Action = "replace conflicting path with absolute link"
	}
	if options.Confirm == nil || !options.Confirm(preview) {
		return preview, ErrNotConfirmed
	}

	// Re-check the source and destination after confirmation to close the
	// concurrent-edit window. Capture happens only after confirmation so a
	// declined preview leaves no journal backup behind.
	if err := verifySource(source); err != nil {
		return preview, err
	}
	conflict, err = destinationConflict(destination, source)
	if err != nil {
		return preview, err
	}
	if conflict && (options.Conflict != ConflictReplace || !options.Force) {
		return preview, ErrUnsafePath
	}
	before, err := options.Journal.Capture([]string{destination})
	if err != nil {
		return preview, err
	}
	if options.BeforePublish != nil {
		if err := options.BeforePublish(); err != nil {
			return preview, err
		}
	}
	if conflict && !matchesSnapshot(before[0]) {
		return preview, ErrUnsafePath
	}
	if err := publishLink(destination, source, conflict); err != nil {
		// A rejected late conflict has not mutated the destination; restoring an
		// empty snapshot here would wrongly delete the unmanaged file that won
		// the race.
		if conflict {
			return preview, errors.Join(err, options.Journal.Restore(before))
		}
		return preview, err
	}
	after, err := options.Journal.Capture([]string{destination})
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
	return current.IsDir() == backup.IsDir()
}

func safePlacementPaths(resource resource.ManagedResource, destination string) (string, string, error) {
	if resource.Provenance.Source == "" || destination == "" {
		return "", "", fmt.Errorf("%w: source and destination are required", ErrUnsafePath)
	}
	source, err := filepath.Abs(resource.Provenance.Source)
	if err != nil {
		return "", "", fmt.Errorf("%w: source and destination are required", ErrUnsafePath)
	}
	destination, err = filepath.Abs(destination)
	if err != nil || source == destination {
		return "", "", fmt.Errorf("%w: source and destination must differ", ErrUnsafePath)
	}
	if filepath.Base(destination) != resource.ID {
		return "", "", fmt.Errorf("%w: destination basename does not match resource identifier", ErrUnsafePath)
	}
	return source, destination, nil
}

func verifySource(source string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect resource source: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: resource source must remain a directory", ErrUnsafePath)
	}
	return nil
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

func publishLink(destination, source string, replace bool) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if replace {
		if err := os.RemoveAll(destination); err != nil {
			return err
		}
	} else if target, err := os.Readlink(destination); err == nil && target == source {
		return nil
	} else if _, err := os.Lstat(destination); err == nil {
		return ErrUnsafePath
	} else if !os.IsNotExist(err) {
		return err
	}
	// Symlink itself is an atomic create and refuses an existing destination,
	// unlike Rename which would overwrite a late unmanaged path.
	return os.Symlink(source, destination)
}
