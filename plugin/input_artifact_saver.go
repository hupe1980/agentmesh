package plugin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/hupe1980/agentmesh/core"
)

// InputArtifactSaver rewrites raw file parts into artifact-backed URIs before a run starts.
type InputArtifactSaver struct {
	Noop
}

// NewInputArtifactSaver creates a plugin that stores raw user file parts as artifacts and
// replaces them with URI references.
func NewInputArtifactSaver() *InputArtifactSaver {
	return &InputArtifactSaver{}
}

// OnUserParts scans user parts for raw file blobs, persists them as artifacts, and returns
// a replacement slice containing URI-backed file parts. It returns nil when no changes are made.
func (pl *InputArtifactSaver) OnUserParts(
	ctx context.Context,
	reqCtx core.RequestContext,
	userParts []core.Part,
) ([]core.Part, error) {
	if len(userParts) == 0 {
		return nil, nil
	}

	updated := make([]core.Part, 0, len(userParts))
	var changed bool

	for idx, part := range userParts {
		filePart, ok := part.(*core.FilePart)
		if !ok {
			updated = append(updated, part)
			continue
		}

		if replacement, err := pl.handleFilePart(ctx, reqCtx, filePart, idx); err != nil {
			return nil, err
		} else if replacement != nil {
			updated = append(updated, replacement...)
			changed = true
			continue
		}

		updated = append(updated, part)
	}

	if !changed {
		return nil, nil
	}

	return updated, nil
}

// handleFilePart processes individual file parts and determines if they need to be saved as artifacts.
func (pl *InputArtifactSaver) handleFilePart(
	ctx context.Context,
	reqCtx core.RequestContext,
	fp *core.FilePart,
	index int,
) ([]core.Part, error) {
	switch fp.File.(type) {
	case *core.FileRawBytes, *core.FileBase64:
		return pl.saveBlobAsArtifact(ctx, reqCtx, fp, index)
	default:
		return nil, nil
	}
}

// saveBlobAsArtifact saves a file part as an artifact and returns a replacement part.
func (pl *InputArtifactSaver) saveBlobAsArtifact(
	ctx context.Context,
	reqCtx core.RequestContext,
	fp *core.FilePart,
	index int,
) ([]core.Part, error) {
	name := fp.Name
	if name == "" {
		name = fmt.Sprintf("upload-%s-%d", uuid.NewString(), index)
	}

	if err := reqCtx.SaveArtifact(ctx, name, fp); err != nil {
		return nil, fmt.Errorf("artifact: failed to save input blob '%s': %w", name, err)
	}

	return []core.Part{
		&core.FilePart{
			File:     &core.FileURI{URI: "artifact:" + name},
			MimeType: fp.MimeType,
			Name:     fp.Name,
		},
	}, nil
}

// ensure InputArtifactSaver implements core.Plugin
var _ core.Plugin = (*InputArtifactSaver)(nil)
