package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/infracost/proto/gen/go/infracost/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// recordingParser is a ParserServiceClient that records the IdentifyProjects
// requests it receives and claims every directory.
type recordingParser struct {
	pb.ParserServiceClient
	requests []*pb.IdentifyProjectsRequest
}

func (r *recordingParser) IdentifyProjects(_ context.Context, in *pb.IdentifyProjectsRequest, _ ...grpc.CallOption) (*pb.IdentifyProjectsResponse, error) {
	r.requests = append(r.requests, in)
	return &pb.IdentifyProjectsResponse{Directory: true}, nil
}

func TestIdentifyDirectory_PassesRepoRoot(t *testing.T) {
	tests := []struct {
		name     string
		repoRoot string
	}{
		{"repo root known", "/repo"},
		{"repo root unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &recordingParser{}
			identifier := &Identifier{plugins: []*Plugin{{
				info:   &pb.GetPluginInfoResponse{Name: "fake"},
				parser: parser,
			}}}

			result := identifier.IdentifyDirectory(context.Background(), "/repo/app", tt.repoRoot, false, []string{"prod"})
			require.NotNil(t, result)

			require.Len(t, parser.requests, 1)
			assert.Equal(t, "/repo/app", parser.requests[0].Directory)
			assert.Equal(t, tt.repoRoot, parser.requests[0].RepoDirectory)
			assert.Equal(t, []string{"prod"}, parser.requests[0].EnvironmentNames)
		})
	}
}

// seedingParser claims every directory with a seed blob, and records the
// IdentifyEnvironments requests it receives.
type seedingParser struct {
	pb.ParserServiceClient
	seed                []byte
	environmentRequests []*pb.IdentifyEnvironmentsRequest
}

func (s *seedingParser) IdentifyProjects(context.Context, *pb.IdentifyProjectsRequest, ...grpc.CallOption) (*pb.IdentifyProjectsResponse, error) {
	return &pb.IdentifyProjectsResponse{Directory: true, RawOptions: s.seed}, nil
}

func (s *seedingParser) IdentifyEnvironments(_ context.Context, in *pb.IdentifyEnvironmentsRequest, _ ...grpc.CallOption) (*pb.IdentifyEnvironmentsResponse, error) {
	s.environmentRequests = append(s.environmentRequests, in)
	return &pb.IdentifyEnvironmentsResponse{}, nil
}

// The seed blob a plugin's IdentifyProjects returns for a directory project
// comes back to its IdentifyEnvironments for that project, as the proto
// documents.
func TestIdentifier_ForwardsSeedRawOptions(t *testing.T) {
	parser := &seedingParser{seed: []byte(`{"seed":true}`)}
	identifier := &Identifier{plugins: []*Plugin{{
		info:   &pb.GetPluginInfoResponse{Name: "fake"},
		parser: parser,
	}}}

	result := identifier.IdentifyDirectory(context.Background(), "/repo/app", "/repo", false, nil)
	require.NotNil(t, result)
	assert.Equal(t, parser.seed, result.RawOptions)

	_, authoritative, err := identifier.IdentifyEnvironments(context.Background(), "/repo/app", "fake", nil, result.RawOptions, []string{"prod"})
	require.NoError(t, err)
	assert.True(t, authoritative)

	require.Len(t, parser.environmentRequests, 1)
	assert.Equal(t, parser.seed, parser.environmentRequests[0].RawOptions)
	assert.Equal(t, []string{"prod"}, parser.environmentRequests[0].EnvironmentNames)
}

func TestCanonicalPaths(t *testing.T) {
	// A symlink to the repo stands in for e.g. macOS /tmp -> /private/tmp: the tree walk resolves
	// dir, but the repo root it was given is still the symlinked path.
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "app"), 0o755))
	require.NoError(t, os.Mkdir(repo+"-other", 0o755))
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(outside, 0o755))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(repo, link))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "escape")))

	resolvedRepo, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	resolvedApp := filepath.Join(resolvedRepo, "app")

	tests := []struct {
		name         string
		dir          string
		repoRoot     string
		wantDir      string
		wantRepoRoot string
		wantOK       bool
	}{
		{"same form", filepath.Join(repo, "app"), repo, resolvedApp, resolvedRepo, true},
		{"dir is the root", repo, repo, resolvedRepo, resolvedRepo, true},
		{"resolved dir, symlinked root", resolvedApp, link, resolvedApp, resolvedRepo, true},
		{"symlinked dir, resolved root", filepath.Join(link, "app"), resolvedRepo, resolvedApp, resolvedRepo, true},
		{"uncleaned paths", repo + "/./app/", repo + "/", resolvedApp, resolvedRepo, true},
		{"symlink out of the repo", filepath.Join(repo, "escape"), repo, filepath.Join(filepath.Dir(resolvedRepo), "outside"), resolvedRepo, false},
		{"sibling sharing a prefix", repo + "-other", repo, resolvedRepo + "-other", resolvedRepo, false},
		{"root unknown", filepath.Join(repo, "app"), "", resolvedApp, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, repoRoot, ok := canonicalPaths(tt.dir, tt.repoRoot)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantDir, dir)
			assert.Equal(t, tt.wantRepoRoot, repoRoot)
		})
	}
}

// A directory that resolves outside the repository is skipped: no plugin is asked about it.
func TestIdentifyDirectory_SkipsDirOutsideRepo(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(repo, 0o755))
	require.NoError(t, os.Mkdir(outside, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "escape")))

	parser := &recordingParser{}
	identifier := &Identifier{plugins: []*Plugin{{
		info:   &pb.GetPluginInfoResponse{Name: "fake"},
		parser: parser,
	}}}

	assert.Nil(t, identifier.IdentifyDirectory(context.Background(), filepath.Join(repo, "escape"), repo, false, nil))
	assert.Empty(t, parser.requests)
}
