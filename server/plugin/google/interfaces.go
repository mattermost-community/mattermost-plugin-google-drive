package google

import (
	"context"

	"golang.org/x/oauth2"
	driveV2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/googleapi"
)

type ClientInterface interface {
	NewDriveService(ctx context.Context, userID string) (DriveInterface, error)
	NewDriveV2Service(ctx context.Context, userID string) (DriveV2Interface, error)
	NewDocsService(ctx context.Context, userID string) (*DocsService, error)
	NewSlidesService(ctx context.Context, userID string) (*SlidesService, error)
	NewSheetsService(ctx context.Context, userID string) (*SheetsService, error)
	NewDriveActivityService(ctx context.Context, userID string) (DriveActivityInterface, error)
	GetGoogleUserToken(userID string) (*oauth2.Token, error)
	ReloadRateLimits(newQueriesPerMinute int, newBurstSize int)
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
