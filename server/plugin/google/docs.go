package google

import (
	"google.golang.org/api/docs/v1"
)

type DocsService struct {
	service *docs.Service
	GoogleServiceBase
}

func (ds *DocsService) Create(doc *docs.Document) (*docs.Document, error) {
	d, err := ds.service.Documents.Create(doc).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return d, nil
}
