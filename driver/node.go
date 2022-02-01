package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{NodeId: d.nodeID}, nil
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability missing in request")
	}
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing in request")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath not provided")
	}

	req.GetVolumeCapability().GetMount().GetMountFlags()
	log.Debug().Interface("req", req).Msg("Getting ISCSI info from request")
	iscsiInfo, err := getISCSIInfo(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	libConfigPath := d.getISCSILibConfigPath(req.GetVolumeId())
	log.Debug().Str("config_path", libConfigPath).Msg("Generated lib config path")
	diskMounter := getISCSIDiskMounter(iscsiInfo, req)

	util := &ISCSIUtil{}
	log.Debug().Msg("Attaching disk")
	if _, err = util.AttachDisk(*diskMounter, libConfigPath); err != nil {
		log.Error().Err(err).Msg("Failed to attach disk")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	libConfigPath := d.getISCSILibConfigPath(req.GetVolumeId())
	log.Debug().Str("config_path", libConfigPath).Msg("Generated lib config path")
	diskUnmounter := getISCSIDiskUnmounter(req)

	iscsiutil := &ISCSIUtil{}
	log.Debug().Msg("Detaching disk")
	if err := iscsiutil.DetachDisk(*diskUnmounter, targetPath, libConfigPath); err != nil {
		log.Error().Err(err).Msg("Failed to unattach disk")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
