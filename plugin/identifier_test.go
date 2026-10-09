package plugin

import (
	"context"
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
