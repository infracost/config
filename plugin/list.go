package plugin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	pb "github.com/infracost/proto/gen/go/infracost/plugin"
	"google.golang.org/grpc"
)

const (
	handshakeMagicCookieKey   = "INFRACOST_PLUGIN"
	handshakeMagicCookieValue = "de8c7e96-497c-4168-80c4-fc875c8ce764"
	handshakeProtocolVersion  = 1
	dispenseName              = "plugin"
)

var handshakeConfig = goplugin.HandshakeConfig{
	ProtocolVersion:  handshakeProtocolVersion,
	MagicCookieKey:   handshakeMagicCookieKey,
	MagicCookieValue: handshakeMagicCookieValue,
}

type Manager struct {
	dir string
}

// NewManager creates a new manager using the plugin directory at dir - if dir is empty, the manager will default to os.UserCacheDir()/infracost/plugins
func NewManager(dir string) (*Manager, error) {
	if dir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(cacheDir, "infracost", "plugins")
	}
	return &Manager{dir: dir}, nil
}

// List launches every binary in the plugin directory, calls GetPluginInfo on
// each, and returns those that report PluginType_PARSER. Entries that fail to
// launch or aren't parser plugins are skipped. The caller is responsible for
// invoking Close on each returned Plugin to terminate its subprocess.
func (m *Manager) List(ctx context.Context) ([]*Plugin, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []*Plugin
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		p, err := connect(filepath.Join(m.dir, entry.Name()))
		if err != nil {
			continue
		}

		info, err := p.plugin.GetPluginInfo(ctx, &pb.GetPluginInfoRequest{})
		if err != nil || info == nil || info.GetType() != pb.PluginType_PARSER {
			p.Close()
			continue
		}

		parserConfig, err := p.parser.GetParserConfig(ctx, &pb.GetParserConfigRequest{})
		if err != nil || parserConfig == nil {
			p.Close()
			continue
		}

		p.parserConfig = parserConfig
		p.info = info
		plugins = append(plugins, p)
	}

	return plugins, nil
}

// Plugin is a launched parser plugin subprocess, retained so its
// ParserService can be invoked. Close must be called to terminate it.
type Plugin struct {
	info         *pb.GetPluginInfoResponse
	parserConfig *pb.GetParserConfigResponse

	plugin pb.PluginServiceClient
	parser pb.ParserServiceClient
	kill   func()
}

// IdentifyProjects calls the plugin's ParserService.IdentifyProjects RPC for
// the given directory.
func (p *Plugin) IdentifyProjects(ctx context.Context, dir string) (*pb.IdentifyProjectsResponse, error) {
	return p.parser.IdentifyProjects(ctx, &pb.IdentifyProjectsRequest{Directory: dir})
}

func (p *Plugin) GetInfo() *pb.GetPluginInfoResponse {
	return p.info
}

func (p *Plugin) GetParserConfig() *pb.GetParserConfigResponse {
	return p.parserConfig
}

// Close terminates the plugin subprocess.
func (p *Plugin) Close() {
	if p.kill != nil {
		p.kill()
	}
}

type grpcPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
}

func (grpcPlugin) GRPCServer(*goplugin.GRPCBroker, *grpc.Server) error {
	return errors.New("not implemented")
}

func (grpcPlugin) GRPCClient(_ context.Context, _ *goplugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	return conn, nil
}

func connect(path string) (*Plugin, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshakeConfig,
		Plugins:          map[string]goplugin.Plugin{dispenseName: grpcPlugin{}},
		Cmd:              exec.Command(path),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           hclog.NewNullLogger(),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, err
	}

	raw, err := rpcClient.Dispense(dispenseName)
	if err != nil {
		client.Kill()
		return nil, err
	}

	conn, ok := raw.(*grpc.ClientConn)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("unexpected dispensed type %T", raw)
	}

	return &Plugin{
		plugin: pb.NewPluginServiceClient(conn),
		parser: pb.NewParserServiceClient(conn),
		kill:   client.Kill,
	}, nil
}
