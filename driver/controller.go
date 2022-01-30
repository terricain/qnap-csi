package driver

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
	"time"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

const (
	// minimumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is smaller than what we support
	minimumVolumeSizeInBytes int64 = 1 * giB

	// maximumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is larger than what we support
	maximumVolumeSizeInBytes int64 = 128 * giB

	// defaultVolumeSizeInBytes is used when the user did not provide a size or
	// the size they provided did not satisfy our requirements
	defaultVolumeSizeInBytes int64 = 16 * giB

	// Volume prefix
	DefaultVolumePrefix string = "csi"
)

var (
	// we only support accessModes.ReadWriteOnce for iscsi volumes
	// will change if we support NFS
	supportedAccessMode = &csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	if violations := validateCapabilities(req.VolumeCapabilities); len(violations) > 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("volume capabilities cannot be satisified: %s", strings.Join(violations, "; ")))
	}

	log.Debug().Msg("Starting create volume request")

	size, err := extractStorage(req.CapacityRange)
	log.Debug().Int64("raw_size_requested_bytes", size).Msg("Raw size requested in bytes")
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}
	sizeGB := size / (1 * giB)
	log.Debug().Int64("raw_size_gib", sizeGB).Msg("Raw size requested in gigabytes")
	name := cleanISCSIName(req.Name)

	if err = d.client.Login(); err != nil {
		log.Error().Err(err).Msg("Failed to login to NAS")
		return nil, status.Error(codes.Internal, "Failed to login to NAS")
	}

	// Check volume doesnt already have a target for it
	targetList, err := d.client.GetStorageISCSITargetList()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get list of ISCSI targets")
		return nil, status.Error(codes.Internal, "Failed to get list of ISCSI targets")
	}
	for _, target := range targetList.Targets {
		if target.Name == name {
			return nil, status.Error(codes.AlreadyExists, "Volume already exists")
		}
	}

	// By now the volume should be ok to create, so we need a LUN, a target, an initiator, attach lun to target
	// then wait for lun to be ready
	targetIndex, err := d.client.CreateStorageISCSITarget(name, false, false, true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create ISCSI target")
		return nil, status.Error(codes.Internal, "Failed to create ISCSI target")
	}

	if err = d.client.CreateStorageISCSIInitiator(targetIndex, false, "", "", false, "", ""); err != nil {
		_ = d.client.DeleteStorageISCSITarget(targetIndex)
		log.Error().Err(err).Msg("Failed to create ISCSI initator")
		return nil, status.Error(codes.Internal, "Failed to create ISCSI initator")
	}

	// Create LUN
	log.Debug().Msg("Creating LUN")
	block, err := d.client.CreateStorageISCSIBlockLUN(name, d.storagePoolID, int(sizeGB), false, 512, false, false, false, false)
	if err != nil {
		_ = d.client.DeleteStorageISCSITarget(targetIndex)
		log.Error().Err(err).Msg("Failed to create ISCSI Block based LUN")
		return nil, status.Error(codes.Internal, "Failed to create ISCSI Block based LUN")
	}

	log.Debug().Msg("Waiting for LUN")
	// Lets wait for the lun to be ready
	for {
		lunInfo, lunErr := d.client.GetStorageISCSILun(block.Result)
		if lunErr != nil {
			log.Error().Err(lunErr).Msg("Failed to get ISCSI Block based LUN readiness")
			return nil, status.Error(codes.Internal, "Failed to get ISCSI Block based LUN readiness")
		}
		if lunInfo.StatusString() == "creating" {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	log.Debug().Msg("Attaching Target to LUN")
	if err = d.client.AttachStorageISCSITargetLUN(block.Result, targetIndex); err != nil {
		_ = d.client.DeleteStorageISCSITarget(targetIndex)
		_ = d.client.DeleteStorageISCSIBlockLUN(block.Result, false)
		log.Error().Err(err).Msg("Failed to associate LUN with ISCSI target")
		return nil, status.Error(codes.Internal, "Failed to associate LUN with ISCSI target")
	}

	targetList, err = d.client.GetStorageISCSITargetList()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get list of ISCSI targets")
		return nil, status.Error(codes.Internal, "Failed to get list of ISCSI targets")
	}
	var iqn = ""
	for _, target := range targetList.Targets {
		if target.Name == name {
			iqn = target.IQN
			break
		}
	}
	if iqn == "" {
		log.Error().Err(err).Msg("Failed to get ISCSI IQN")
		return nil, status.Error(codes.Internal, "Failed to get ISCSI IQN")
	}

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      name,
			CapacityBytes: size,
			VolumeContext: map[string]string{
				"targetPortal": d.portal,
				"iqn": iqn,
				"lun": "0",
				"portals": "[]",
			},
		},
	}

	return resp, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	if err := d.client.Login(); err != nil {
		log.Error().Err(err).Msg("Failed to login to NAS")
		return nil, status.Error(codes.Internal, "Failed to login to NAS")
	}

	targetList, err := d.client.GetStorageISCSITargetList()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get list of ISCSI targets")
		return nil, status.Error(codes.Internal, "Failed to get list of ISCSI targets")
	}
	for _, target := range targetList.Targets {
		if target.Name == req.VolumeId {
			if err = d.client.DeleteStorageISCSITarget(target.TargetIndex); err != nil {
				log.Error().Err(err).Msg("Failed to delete ISCSI target")
				return nil, status.Error(codes.Internal, "Failed to delete ISCSI target")
			}

			for _, targetLUNID := range target.TargetLUNs {
				if err = d.client.DeleteStorageISCSIBlockLUN(targetLUNID, false); err != nil {
					log.Error().Err(err).Msg("Failed to delete ISCSI Block based LUN")
					return nil, status.Error(codes.Internal, "Failed to delete ISCSI Block based LUN")
				}
			}

			return &csi.DeleteVolumeResponse{}, nil
		}
	}

	// Volume not found, so yolo
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID must be provided")
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	if err := d.client.Login(); err != nil {
		log.Error().Err(err).Msg("Failed to login to NAS")
		return nil, status.Error(codes.Internal, "Failed to login to NAS")
	}

	resp, err := d.client.GetStorageISCSITargetList()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get list of ISCSI targets")
		return nil, status.Error(codes.Internal, "Failed to get list of ISCSI targets")
	}

	found := false
	for _, target := range resp.Targets {
		if target.Name == req.VolumeId {
			found = true
			break
		}
	}
	if !found {
		return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities Volume ID %s not found", req.VolumeId)
	}

	// Literally the only thing we support: so no point in checking the actual params ;-)
	result := &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: supportedAccessMode,
				},
			},
		},
	}


	return result, nil
}

func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// So volume id's can be backfilled, so the plan is to base64 encode a list of "seen" numbers,
	if err := d.client.Login(); err != nil {
		log.Error().Err(err).Msg("Failed to login to NAS")
		return nil, status.Error(codes.Internal, "Failed to login to NAS")
	}

	resp, err := d.client.GetStorageISCSITargetList()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get list of ISCSI targets")
		return nil, status.Error(codes.Internal, "Failed to get list of ISCSI targets")
	}

	var seenIds *sets.Int
	if req.GetStartingToken() != "" {
		seenIds, err = base64SeenIndexesToMap(req.GetStartingToken())
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse list volume starting token, ignoring")
			tmp := sets.NewInt()
			seenIds = &tmp
		}
	}

	entries := make([]*csi.ListVolumesResponse_Entry, 0)
	maxEntries := req.GetMaxEntries()
	nextToken := ""
	if maxEntries == 0 {
		maxEntries = int32(len(resp.Targets))
	}

	for _, target := range resp.Targets {
		if seenIds.Has(target.TargetIndex) {
			// Seen this volume before,
			continue
		}

		// We've not seen the item here
		if int32(len(entries)) >= maxEntries {
			// There are more entries to add, but we're full
			nextToken = seenIndexesToBase64String(seenIds.List())
			break
		}

		// entries, not at limit, add one
		// TODO(more) go find the LUN attached to the target and get capacity
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume:               &csi.Volume{
				VolumeId:             target.Name,
				// Could use volume context for specific values,
			},
			Status:               nil,
		})
		seenIds.Insert(target.TargetIndex)
	}

	result := &csi.ListVolumesResponse{
		Entries: entries,
		NextToken: nextToken,
	}

	return result, nil
}

func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	if err := d.client.Login(); err != nil {
		log.Error().Err(err).Msg("Failed to login to NAS")
		return nil, status.Error(codes.Internal, "Failed to login to NAS")
	}

	resp, err := d.client.GetStoragePoolSubscription(d.storagePoolID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get storage pool size")
		return nil, status.Error(codes.Internal, "Failed to get storage pool capacity")
	}

	return &csi.GetCapacityResponse{
		AvailableCapacity: int64(resp.PoolSubscription.CapacityBytes),
		MaximumVolumeSize: wrapperspb.Int64(maximumVolumeSizeInBytes),
		MinimumVolumeSize: wrapperspb.Int64(minimumVolumeSizeInBytes),
	}, nil
}

func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	caps := make([]*csi.ControllerServiceCapability, 0)
	for _, currentCap := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		// csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		// csi.ControllerServiceCapability_RPC_GET_VOLUME,
		// csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		// csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
	} {
		caps = append(caps, newCap(currentCap))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	return resp, nil
}




func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// TODO(unimpl)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	// TODO(unimpl)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// TODO(unimpl)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	// TODO(unimpl)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// TODO(unimpl)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	// TODO(unimpl)
	// In theory we dont need this
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// TODO(unimpl)
	// In theory we dont need this
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// validateCapabilities validates the requested capabilities. It returns a list
// of violations which may be empty if no violatons were found.
func validateCapabilities(caps []*csi.VolumeCapability) []string {
	violations := sets.NewString()
	for _, currentCap := range caps {
		if currentCap.GetAccessMode().GetMode() != supportedAccessMode.GetMode() {
			violations.Insert(fmt.Sprintf("unsupported access mode %s", currentCap.GetAccessMode().GetMode().String()))
		}

		accessType := currentCap.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Block:
		case *csi.VolumeCapability_Mount:
		default:
			violations.Insert("unsupported access type")
		}
	}

	return violations.List()
}