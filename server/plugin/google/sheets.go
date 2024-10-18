package google

import (
	"google.golang.org/api/sheets/v4"
)

type SheetsService struct {
	service *sheets.Service
	GoogleServiceBase
}

func (ds *SheetsService) Create(spreadsheet *sheets.Spreadsheet) (*sheets.Spreadsheet, error) {
	p, err := ds.service.Spreadsheets.Create(spreadsheet).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return p, nil
}
