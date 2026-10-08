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
