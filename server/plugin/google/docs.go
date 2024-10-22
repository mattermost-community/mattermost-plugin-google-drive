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
	d, err := ds.service.Documents.Create(doc).Do()
	if err != nil {
		ds.parseGoogleErrors(ctx, err)
		return nil, err
	}
	return d, nil
}
