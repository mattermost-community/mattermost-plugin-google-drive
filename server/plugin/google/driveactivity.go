package google

import (
	"google.golang.org/api/driveactivity/v2"
)

type DriveActivityService struct {
	service *driveactivity.Service
	googleServiceBase
}

func (ds *DriveActivityService) Query(request *driveactivity.QueryDriveActivityRequest) (*driveactivity.QueryDriveActivityResponse, error) {
	p, err := ds.service.Activity.Query(request).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return p, nil
}
