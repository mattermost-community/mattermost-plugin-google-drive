package google

import (
	"context"

	"google.golang.org/api/sheets/v4"
)

type SheetsService struct {
	service *sheets.Service
	googleServiceBase
}

func (ds *SheetsService) Create(ctx context.Context, spreadsheet *sheets.Spreadsheet) (*sheets.Spreadsheet, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	p, err := ds.service.Spreadsheets.Create(spreadsheet).Do()
	if err != nil {
		err = ds.parseGoogleErrors(ctx, err)
		return nil, err
	}
	return p, nil
}
