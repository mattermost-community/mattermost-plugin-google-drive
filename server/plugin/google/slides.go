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
	p, err := ds.service.Presentations.Create(presentation).Do()
	if err != nil {
		ds.parseGoogleErrors(ctx, err)
		return nil, err
	}
	return p, nil
}
