package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AllenMuu/skill-manager/internal/operation"
	"github.com/AllenMuu/skill-manager/internal/resource"
	"github.com/AllenMuu/skill-manager/internal/subagent"
)

// SubAgentFilesystemOptions controls a guarded filesystem-backed SubAgent
// installation or removal. SourceRoot is the managed root for rendered
// representations; it is never treated as user-owned target content.
type SubAgentFilesystemOptions struct {
	Target     Target
	SourceRoot string
	Conflict   ConflictStrategy
	Force      bool
	Journal    *operation.Journal
	Confirm    func(operation.Plan) bool
	// BeforePublish is a test/integration seam invoked after confirmation and
	// before the rendered representation is published.
	BeforePublish func() error
	// BeforeRemove is a test/integration seam invoked after capture and before
	// atomic staging of the destination.
	BeforeRemove func() error
}

// InstallSubAgent renders and installs one canonical SubAgent through its
// target adapter. Pi is intentionally unsupported because it has no verified
// native SubAgent representation.
func InstallSubAgent(definition subagent.Definition, request SubAgentRequest, options SubAgentFilesystemOptions) (operation.Plan, error) {
	target := options.Target
	if target == "" {
		target = ClaudeCode
	}
	a, ok := ForAgent(target)
	if !ok {
		return operation.Plan{}, fmt.Errorf("unsupported target %q", target)
	}
	renderer, ok := a.(SubAgentAdapter)
	if !ok {
		return operation.Plan{}, fmt.Errorf("target %q has no SubAgent adapter", target)
	}
	rendered, err := renderer.PlanSubAgent(definition, request)
	if err != nil {
		return operation.Plan{}, err
	}
	if !rendered.Supported() {
		return operation.Plan{}, fmt.Errorf("%w for %s: unsupported fields=%v capabilities=%v", ErrSubAgentUnsupported, target, rendered.UnsupportedFields, rendered.UnsupportedCapabilities)
	}
	root := options.SourceRoot
	if root == "" {
		root = request.Root
	}
	if root == "" || options.Journal == nil {
		return operation.Plan{}, errors.New("SubAgent filesystem placement requires source root and operation journal")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return operation.Plan{}, err
	}
	ext := ".md"
	if rendered.Format == "codex-toml" {
		ext = ".toml"
	}
	source := filepath.Join(root, ".agent-manager", "subagents", string(target), definition.ID+ext)
	managed := resource.ManagedResource{
		Version: "v1", ID: definition.ID, Kind: resource.SubAgent,
		Provenance:           resource.Provenance{Source: source},
		RequiredCapabilities: append([]resource.Capability(nil), definition.RequiredCapabilities...),
	}
	plan := resource.PlacementPlan{Resource: managed, Destination: rendered.Destination, Conflict: string(options.Conflict), Force: options.Force}
	return PlaceFilesystem(plan, FilesystemPlacementOptions{
		Destination:   rendered.Destination,
		SourceContent: []byte(rendered.Content),
		Conflict:      options.Conflict,
		Force:         options.Force,
		Journal:       options.Journal,
		Confirm:       options.Confirm,
		BeforePublish: options.BeforePublish,
	})
}

// RemoveSubAgent removes only a target representation owned by this manager.
// It refuses ordinary files, directories, and links to a different source.
func RemoveSubAgent(definition subagent.Definition, request SubAgentRequest, options SubAgentFilesystemOptions) (operation.Plan, error) {
	target := options.Target
	if target == "" {
		target = ClaudeCode
	}
	a, ok := ForAgent(target)
	if !ok {
		return operation.Plan{}, fmt.Errorf("unsupported target %q", target)
	}
	renderer, ok := a.(SubAgentAdapter)
	if !ok {
		return operation.Plan{}, fmt.Errorf("target %q has no SubAgent adapter", target)
	}
	rendered, err := renderer.PlanSubAgent(definition, request)
	if err != nil {
		return operation.Plan{}, err
	}
	if !rendered.Supported() {
		return operation.Plan{}, fmt.Errorf("%w for %s: unsupported fields=%v capabilities=%v", ErrSubAgentUnsupported, target, rendered.UnsupportedFields, rendered.UnsupportedCapabilities)
	}
	root := options.SourceRoot
	if root == "" {
		root = request.Root
	}
	ext := ".md"
	if rendered.Format == "codex-toml" {
		ext = ".toml"
	}
	source, err := filepath.Abs(filepath.Join(root, ".agent-manager", "subagents", string(target), definition.ID+ext))
	if err != nil || options.Journal == nil {
		return operation.Plan{}, errors.New("SubAgent filesystem removal requires source root and operation journal")
	}
	destination, err := filepath.Abs(rendered.Destination)
	if err != nil {
		return operation.Plan{}, err
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return operation.Plan{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return operation.Plan{}, ErrUnsafePath
	}
	linked, err := os.Readlink(destination)
	if err != nil || linked != source {
		return operation.Plan{}, ErrUnsafePath
	}
	preview := operation.NewPlan("remove SubAgent")
	preview.ResourceKind = string(resource.SubAgent)
	preview.Changes = []operation.Change{{Path: destination, Action: "remove managed SubAgent"}}
	if options.Confirm == nil || !options.Confirm(preview) {
		return preview, ErrNotConfirmed
	}
	// Re-check ownership after confirmation. Confirmation callbacks may involve
	// user interaction, during which another process can replace the target.
	// Never capture or remove a path that is no longer our managed link.
	current, err := os.Lstat(destination)
	if err != nil {
		return preview, ErrUnsafePath
	}
	if current.Mode()&os.ModeSymlink == 0 {
		return preview, ErrUnsafePath
	}
	linked, err = os.Readlink(destination)
	if err != nil || linked != source {
		return preview, ErrUnsafePath
	}
	before, err := options.Journal.Capture([]string{destination})
	if err != nil {
		return preview, err
	}
	if options.BeforeRemove != nil {
		if err := options.BeforeRemove(); err != nil {
			return preview, err
		}
	}
	stageRoot, err := os.MkdirTemp(filepath.Dir(destination), ".agent-manager-remove-")
	if err != nil {
		return preview, err
	}
	defer os.RemoveAll(stageRoot)
	staged := filepath.Join(stageRoot, "destination")
	if err := os.Rename(destination, staged); err != nil {
		return preview, err
	}
	rollback := func(cause error) error {
		if _, statErr := os.Lstat(destination); os.IsNotExist(statErr) {
			if renameErr := os.Rename(staged, destination); renameErr != nil {
				return errors.Join(cause, renameErr)
			}
			return cause
		}
		return errors.Join(cause, fmt.Errorf("restore staged SubAgent: destination was recreated"))
	}
	stagedInfo, err := os.Lstat(staged)
	if err != nil {
		return preview, rollback(err)
	}
	if stagedInfo.Mode()&os.ModeSymlink == 0 {
		return preview, rollback(ErrUnsafePath)
	}
	stagedTarget, err := os.Readlink(staged)
	if err != nil || stagedTarget != source {
		return preview, rollback(ErrUnsafePath)
	}
	if err := os.Remove(staged); err != nil {
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

// Supported reports whether the render plan can be safely installed.
func (p SubAgentPlan) Supported() bool {
	return p.Destination != "" && p.Content != "" && len(p.UnsupportedFields) == 0 && len(p.UnsupportedCapabilities) == 0
}
