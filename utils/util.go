package utils

import (
	"context"

	"github.com/nspcc-dev/neofs-http-gw/resolver"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
)

// GetContainerID decode container id, if it's not a valid container id
// then trey to resolve name using provided resolver.
func GetContainerID(ctx context.Context, containerID string, resolver resolver.Resolver) (*cid.ID, error) {
	var cnrID cid.ID
	err := cnrID.DecodeString(containerID)
	if err != nil {
		cnrID, err = resolver.Resolve(ctx, containerID)
	}
	return &cnrID, err
}
