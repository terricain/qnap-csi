package main

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)


func main() {
	conn, err := grpc.Dial("unix:///tmp/csi/csi.sock", grpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to listen to socket")
	}
	defer conn.Close()

	client := csi.NewNodeClient(conn)

	//_, err = client.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
	//	VolumeId: "testvol1",
	//	TargetPath: "/tmp/lala",
	//	VolumeCapability: &csi.VolumeCapability{
	//		AccessType: &csi.VolumeCapability_Mount{
	//			Mount: &csi.VolumeCapability_MountVolume{
	//				FsType: "ext4",
	//			},
	//		},
	//		AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
	//
	//	},
	//	Readonly: false,
	//	VolumeContext: map[string]string{
	//		"targetPortal": "172.20.0.5:3260",
	//		"iqn": "iqn.2004-04.com.qnap:ts-1279u-rp:iscsi.testvol1.cbbc56",
	//		"lun": "0",
	//		"portals": "[]",
	//	},
	//})
	_, err = client.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId: "testvol1",
		TargetPath: "/tmp/lala",
	})
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}
