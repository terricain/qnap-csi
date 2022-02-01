package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	"github.com/terrycain/qnap-csi/qnap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	DefaultDriverName = "qnap.terrycain.github.com"
)

var (
	GitTreeState = "not a git tree"
	Commit       string
	Version      string
)

type Driver struct {
	name string

	storagePoolID int
	endpoint      string
	URL           string
	nodeID        string
	username      string
	password      string
	client        *qnap.Client
	isController  bool
	prefix        string
	portal        string
	configDir     string

	srv *grpc.Server

	readyMu sync.Mutex // protects ready
	ready   bool
}

func NewDriver(endpoint, url, username, password string, isController bool, prefix string, nodeID string, portal string, storagePoolID int) (*Driver, error) {
	qnapClient, err := qnap.NewClient(username, password, url)
	if err != nil {
		return nil, err
	}

	return &Driver{
		name:          DefaultDriverName,
		storagePoolID: storagePoolID,
		client:        qnapClient,
		URL:           url,
		isController:  isController,
		endpoint:      endpoint,
		username:      username,
		nodeID:        nodeID,
		password:      password,
		prefix:        prefix,
		portal:        portal,
	}, nil
}

func (d *Driver) Run(ctx context.Context) error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %w", err)
	}

	grpcAddr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		grpcAddr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	}

	// Remove socket if it exists
	if err = os.Remove(grpcAddr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old unix domain socket file %s, error: %w", grpcAddr, err)
	}

	sockPath := path.Dir(u.Path)
	if err = os.MkdirAll(sockPath, 0o750); err != nil {
		return fmt.Errorf("failed to make directories for sock, error: %w", err)
	}
	d.configDir = path.Join(sockPath, "config")
	if err = os.MkdirAll(d.configDir, 0o750); err != nil {
		return fmt.Errorf("failed to make directories for config, error: %w", err)
	}

	grpcListener, err := net.Listen(u.Scheme, grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			log.Error().Err(err).Str("method", info.FullMethod).Msg("method failed")
		}
		return resp, err
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	d.setReady(true)
	log.Info().Str("grpc_addr", grpcAddr).Msg("starting CSI GRPC server")

	var eg errgroup.Group
	eg.Go(func() error {
		go func() {
			<-ctx.Done()
			log.Info().Msg("Server stopped")
			d.setReady(false)
			d.srv.GracefulStop()
		}()
		return d.srv.Serve(grpcListener)
	})

	return eg.Wait()
}

func (d *Driver) setReady(state bool) {
	d.readyMu.Lock()
	defer d.readyMu.Unlock()
	d.ready = state
}

func (d *Driver) getISCSILibConfigPath(id string) string {
	return path.Join(d.configDir, id+".json")
}
