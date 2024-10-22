package google

import (
	"context"

	"google.golang.org/api/driveactivity/v2"
)

type DriveActivityService struct {
	service *driveactivity.Service
	googleServiceBase
}

func (ds *DriveActivityService) Query(ctx context.Context, request *driveactivity.QueryDriveActivityRequest) (*driveactivity.QueryDriveActivityResponse, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	p, err := ds.service.Activity.Query(request).Do()
	if err != nil {
		ds.parseGoogleErrors(ctx, err)
		return nil, err
	}
	return p, nil
}
