package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
)

func TestInputArtifactSaver_RewritesBlobs(t *testing.T) {
	ctx := context.Background()
	saved := make(map[string]core.Part)

	artifactStore := &testutil.ArtifactStoreMock{
		SaveFunc: func(
			_ context.Context,
			_, _, _, fileName string,
			artifact core.Part,
		) error {
			saved[fileName] = artifact
			return nil
		},
	}

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.ArtifactStore = artifactStore
	})

	saver := NewInputArtifactSaver()

	userParts := []core.Part{
		core.NewPartFromText("hello"),
		core.NewPartFromFileRawBytes("doc.txt", []byte("content")),
		core.NewPartFromFileBase64("img.png", "ZmFrZQ=="),
		core.NewPartFromFileURI("existing", "artifact:existing"),
	}

	out, err := saver.OnUserParts(ctx, reqCtx, userParts)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out, len(userParts))

	require.Same(t, userParts[0], out[0])

	fp2, ok := out[1].(*core.FilePart)
	require.True(t, ok)
	require.IsType(t, &core.FileURI{}, fp2.File)
	require.Equal(t, "doc.txt", fp2.Name)
	require.Equal(t, "artifact:doc.txt", fp2.File.(*core.FileURI).URI)

	fp3, ok := out[2].(*core.FilePart)
	require.True(t, ok)
	require.IsType(t, &core.FileURI{}, fp3.File)
	require.Equal(t, "img.png", fp3.Name)
	require.Equal(t, "artifact:img.png", fp3.File.(*core.FileURI).URI)

	require.Same(t, userParts[3], out[3])

	require.Contains(t, saved, "doc.txt")
	require.Contains(t, saved, "img.png")
}

func TestInputArtifactSaver_NoChangesReturnsNil(t *testing.T) {
	ctx := context.Background()
	reqCtx := testutil.NewTestRequestContext()

	saver := NewInputArtifactSaver()

	userParts := []core.Part{
		core.NewPartFromText("hello"),
		core.NewPartFromFileURI("existing", "artifact:existing"),
	}

	out, err := saver.OnUserParts(ctx, reqCtx, userParts)
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestInputArtifactSaver_GeneratesNameWhenMissing(t *testing.T) {
	ctx := context.Background()
	var savedName string

	artifactStore := &testutil.ArtifactStoreMock{
		SaveFunc: func(
			_ context.Context,
			_, _, _, fileName string,
			_ core.Part,
		) error {
			savedName = fileName
			return nil
		},
	}

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.ArtifactStore = artifactStore
	})

	saver := NewInputArtifactSaver()

	userParts := []core.Part{
		&core.FilePart{File: &core.FileRawBytes{Bytes: []byte("content")}},
	}

	out, err := saver.OnUserParts(ctx, reqCtx, userParts)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out, 1)

	require.NotEmpty(t, savedName)
	require.True(t, strings.HasPrefix(savedName, "upload-"))

	fp, ok := out[0].(*core.FilePart)
	require.True(t, ok)
	require.IsType(t, &core.FileURI{}, fp.File)
	require.True(t, strings.HasPrefix(fp.File.(*core.FileURI).URI, "artifact:"))
}
