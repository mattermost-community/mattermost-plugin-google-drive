package google

import (
	"context"

	"google.golang.org/api/slides/v1"
)

type SlidesService struct {
	service *slides.Service
	googleServiceBase
}

func (ds *SlidesService) Create(ctx context.Context, presentation *slides.Presentation) (*slides.Presentation, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	p, err := ds.service.Presentations.Create(presentation).Do()
	if err != nil {
		err = ds.checkForRateLimitErrors(err)
		return nil, err
	}
	return p, nil
}
