package google

import (
	"context"

	oauth2package "golang.org/x/oauth2"
	"google.golang.org/api/docs/v1"
	driveV2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2"
)

type ClientInterface interface {
	NewDriveService(ctx context.Context, userID string) (DriveInterface, error)
	NewDriveV2Service(ctx context.Context, userID string) (DriveV2Interface, error)
	NewDocsService(ctx context.Context, userID string) (DocsInterface, error)
	NewSlidesService(ctx context.Context, userID string) (SlidesInterface, error)
	NewSheetsService(ctx context.Context, userID string) (SheetsInterface, error)
	NewDriveActivityService(ctx context.Context, userID string) (DriveActivityInterface, error)
	GetGoogleUserToken(userID string) (*oauth2package.Token, error)
	ReloadConfigs(newQueriesPerMinute int, newBurstSize int, oauthConfig oauth2.Config)
}

type DriveInterface interface {
	About(ctx context.Context, fields googleapi.Field) (*drive.About, error)
	WatchChannel(ctx context.Context, startPageToken *drive.StartPageToken, requestChannel *drive.Channel) (*drive.Channel, error)
	StopChannel(ctx context.Context, channel *drive.Channel) error
	ChangesList(ctx context.Context, pageToken string) (*drive.ChangeList, error)
	GetStartPageToken(ctx context.Context) (*drive.StartPageToken, error)
	GetComments(ctx context.Context, fileID string, commentID string) (*drive.Comment, error)
	CreateReply(ctx context.Context, fileID string, commentID string, reply *drive.Reply) (*drive.Reply, error)
	CreateFile(ctx context.Context, file *drive.File, fileReader []byte) (*drive.File, error)
	GetFile(ctx context.Context, fileID string) (*drive.File, error)
	CreatePermission(ctx context.Context, fileID string, permission *drive.Permission) (*drive.Permission, error)
}

type DriveV2Interface interface {
	About(ctx context.Context, fields googleapi.Field) (*driveV2.About, error)
}

type DriveActivityInterface interface {
	Query(ctx context.Context, request *driveactivity.QueryDriveActivityRequest) (*driveactivity.QueryDriveActivityResponse, error)
}

type DocsInterface interface {
	Create(ctx context.Context, document *docs.Document) (*docs.Document, error)
}

type SheetsInterface interface {
	Create(ctx context.Context, spreadsheet *sheets.Spreadsheet) (*sheets.Spreadsheet, error)
}

type SlidesInterface interface {
	Create(ctx context.Context, presentation *slides.Presentation) (*slides.Presentation, error)
}
