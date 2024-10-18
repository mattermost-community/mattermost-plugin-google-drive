package google

import (
	"google.golang.org/api/slides/v1"
)

type SlidesService struct {
	service *slides.Service
	GoogleServiceBase
}

func (ds *SlidesService) Create(presentation *slides.Presentation) (*slides.Presentation, error) {
	p, err := ds.service.Presentations.Create(presentation).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return p, nil
}
