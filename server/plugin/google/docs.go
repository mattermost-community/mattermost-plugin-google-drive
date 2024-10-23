package google

import (
	"context"

	"google.golang.org/api/docs/v1"
)

type DocsService struct {
	service *docs.Service
	googleServiceBase
}

func (ds *DocsService) Create(ctx context.Context, doc *docs.Document) (*docs.Document, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	d, err := ds.service.Documents.Create(doc).Do()
	if err != nil {
		err = ds.parseGoogleErrors(ctx, err)
		return nil, err
	}
	return d, nil
}
