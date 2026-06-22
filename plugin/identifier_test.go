package plugin

import (
	"context"
	"testing"

	"github.com/infracost/config/types"
	pb "github.com/infracost/proto/gen/go/infracost/plugin"
	"google.golang.org/grpc"
)

// fakeParser is a ParserServiceClient whose IdentifyProjects returns a canned
// response, ignoring the requested directory.
type fakeParser struct {
	resp *pb.IdentifyProjectsResponse
}

func (f fakeParser) GetParserConfig(context.Context, *pb.GetParserConfigRequest, ...grpc.CallOption) (*pb.GetParserConfigResponse, error) {
	return &pb.GetParserConfigResponse{}, nil
}
func (f fakeParser) IdentifyProjects(context.Context, *pb.IdentifyProjectsRequest, ...grpc.CallOption) (*pb.IdentifyProjectsResponse, error) {
	return f.resp, nil
}
func (f fakeParser) Parse(context.Context, *pb.ParseRequest, ...grpc.CallOption) (*pb.ParseResponse, error) {
	return &pb.ParseResponse{}, nil
}

func testPlugin(name string, resp *pb.IdentifyProjectsResponse) *Plugin {
	projectType := name
	return &Plugin{
		info:         &pb.GetPluginInfoResponse{Name: name},
		parserConfig: &pb.GetParserConfigResponse{ConfigFileProjectType: &projectType},
		parser:       fakeParser{resp: resp},
	}
}

func TestIdentifyDirectory_DirectoryAndFilesCoexist(t *testing.T) {
	// terraform claims the whole directory; kubernetes + appcode each claim
	// their own files in the same directory. All three must surface.
	id := &Identifier{plugins: []*Plugin{
		testPlugin("terraform", &pb.IdentifyProjectsResponse{Directory: true}),
		testPlugin("kubernetes", &pb.IdentifyProjectsResponse{Files: []string{"deploy.yaml"}}),
		testPlugin("appcode", &pb.IdentifyProjectsResponse{Files: []string{"agent.ts"}}),
	}}

	got := id.IdentifyDirectory(context.Background(), "/some/dir")
	if got == nil {
		t.Fatal("expected a result, got nil")
	}
	if got.DirectoryType != types.ProjectType("terraform") {
		t.Errorf("DirectoryType = %q, want terraform", got.DirectoryType)
	}
	if got.FileTypes["deploy.yaml"] != types.ProjectType("kubernetes") {
		t.Errorf("deploy.yaml = %q, want kubernetes", got.FileTypes["deploy.yaml"])
	}
	if got.FileTypes["agent.ts"] != types.ProjectType("appcode") {
		t.Errorf("agent.ts = %q, want appcode", got.FileTypes["agent.ts"])
	}
}

func TestIdentifyDirectory_FirstDirectoryClaimWins(t *testing.T) {
	// Two whole-directory claimers (terragrunt + terraform): winner-takes-all in
	// priority order, no second directory project. Mirrors the terragrunt-wraps-
	// terraform exclusivity.
	id := &Identifier{plugins: []*Plugin{
		testPlugin("terragrunt", &pb.IdentifyProjectsResponse{Directory: true}),
		testPlugin("terraform", &pb.IdentifyProjectsResponse{Directory: true}),
	}}

	got := id.IdentifyDirectory(context.Background(), "/some/dir")
	if got == nil {
		t.Fatal("expected a result, got nil")
	}
	if got.DirectoryType != types.ProjectType("terragrunt") {
		t.Errorf("DirectoryType = %q, want terragrunt (first claimer)", got.DirectoryType)
	}
	if len(got.FileTypes) != 0 {
		t.Errorf("FileTypes = %v, want empty", got.FileTypes)
	}
}

func TestIdentifyDirectory_NothingIdentified(t *testing.T) {
	id := &Identifier{plugins: []*Plugin{
		testPlugin("terraform", &pb.IdentifyProjectsResponse{}),
	}}
	if got := id.IdentifyDirectory(context.Background(), "/some/dir"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestIdentifyDirectory_HigherPriorityWinsFileConflict(t *testing.T) {
	// Two plugins claim the same file; the first (higher-priority) wins.
	id := &Identifier{plugins: []*Plugin{
		testPlugin("cloudformation", &pb.IdentifyProjectsResponse{Files: []string{"stack.yaml"}}),
		testPlugin("kubernetes", &pb.IdentifyProjectsResponse{Files: []string{"stack.yaml"}}),
	}}
	got := id.IdentifyDirectory(context.Background(), "/some/dir")
	if got == nil {
		t.Fatal("expected a result, got nil")
	}
	if got.FileTypes["stack.yaml"] != types.ProjectType("cloudformation") {
		t.Errorf("stack.yaml = %q, want cloudformation (higher priority)", got.FileTypes["stack.yaml"])
	}
}
