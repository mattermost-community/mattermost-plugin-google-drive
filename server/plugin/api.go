package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gorilla/mux"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/flow"
	"github.com/pkg/errors"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/pluginapi"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

func deferClose(c io.Closer) {
	_ = c.Close()
}

// ResponseType indicates type of response returned by api
type ResponseType string

const (
	// ResponseTypeJSON indicates that response type is json
	ResponseTypeJSON ResponseType = "JSON_RESPONSE"
	// ResponseTypePlain indicates that response type is text plain
	ResponseTypePlain ResponseType = "TEXT_RESPONSE"

	APIErrorIDNotConnected  = "not_connected"
	requestTimeout          = 60 * time.Second
	stateRandomStringLength = 15
)

type Context struct {
	Ctx    context.Context
	UserID string
	Log    logger.Logger
}

type FileCreationRequest struct {
	Name           string `json:"name"`
	FileAccess     string `json:"file_access"`
	Message        string `json:"message"`
	ShareInChannel bool   `json:"share_in_channel"`
}

type APIErrorResponse struct {
	ID         string `json:"id"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

func (e *APIErrorResponse) Error() string {
	return e.Message
}

// DialogErrorResponse is an error response used in interactive dialogs
type DialogErrorResponse struct {
	Error      string `json:"error"`
	StatusCode int    `json:"status_code"`
}

// HTTPHandlerFuncWithContext is http.HandleFunc but with a Context attached
type HTTPHandlerFuncWithContext func(c *Context, w http.ResponseWriter, r *http.Request)

func (p *Plugin) createContext(_ http.ResponseWriter, r *http.Request) (*Context, context.CancelFunc) {
	userID := r.Header.Get("Mattermost-User-ID")

	logger := logger.New(p.API).With(logger.LogContext{
		"userid": userID,
	})

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

	context := &Context{
		Ctx:    ctx,
		UserID: userID,
		Log:    logger,
	}

	return context, cancel
}

func (p *Plugin) attachContext(handler HTTPHandlerFuncWithContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		context, cancel := p.createContext(w, r)
		defer cancel()

		handler(context, w, r)
	}
}

func (p *Plugin) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if x := recover(); x != nil {
				p.Client.Log.Warn("Recovered from a panic",
					"url", r.URL.String(),
					"error", x,
					"stack", string(debug.Stack()))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) checkConfigured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config := p.getConfiguration()

		if err := config.IsValid(); err != nil {
			http.Error(w, "This plugin is not configured.", http.StatusNotImplemented)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) writeAPIError(w http.ResponseWriter, err *APIErrorResponse) {
	b, _ := json.Marshal(err)
	w.WriteHeader(err.StatusCode)
	if _, err := w.Write(b); err != nil {
		p.Client.Log.Warn("Can't write api error http response", "err", err.Error())
	}
}

func (p *Plugin) writeInteractiveDialogError(w http.ResponseWriter, errResponse DialogErrorResponse) {
	w.WriteHeader(errResponse.StatusCode)
	if errResponse.Error == "" {
		errResponse.Error = "Something went wrong, please contact your system administrator"
	}
	if err := json.NewEncoder(w).Encode(errResponse); err != nil {
		p.Client.Log.Warn("Can't write api error http response", "err", err.Error())
	}
}

func (p *Plugin) checkAuth(handler http.HandlerFunc, responseType ResponseType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			switch responseType {
			case ResponseTypeJSON:
				p.writeAPIError(w, &APIErrorResponse{ID: "", Message: "Not authorized.", StatusCode: http.StatusUnauthorized})
			case ResponseTypePlain:
				http.Error(w, "Not authorized", http.StatusUnauthorized)
			default:
				p.Client.Log.Debug("Unknown ResponseType detected")
			}
			return
		}

		handler(w, r)
	}
}

func (p *Plugin) connectUserToGoogle(c *Context, w http.ResponseWriter, r *http.Request) {
	state := fmt.Sprintf("%v_%v", mattermostModel.NewId()[0:stateRandomStringLength], c.UserID)

	if err := p.KVStore.StoreOAuthStateToken(state, state); err != nil {
		c.Log.WithError(err).Warnf("Can't store state oauth2")
		http.Error(w, "can't store state oauth2", http.StatusInternalServerError)
		return
	}

	url := p.oauthConfig.AuthCodeURL(state)

	ch := p.oauthBroker.SubscribeOAuthComplete(c.UserID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		var errorMsg string
		select {
		case err := <-ch:
			if err != nil {
				errorMsg = err.Error()
			}
		case <-ctx.Done():
			errorMsg = "Timed out waiting for OAuth connection. Please check if the SiteURL is correct."
		}

		if errorMsg != "" {
			_, err := p.poster.DMWithAttachments(c.UserID, &mattermostModel.SlackAttachment{
				Text:  fmt.Sprintf("There was an error connecting to your Google account: `%s` Please double check your configuration.", errorMsg),
				Color: string(flow.ColorDanger),
			})
			if err != nil {
				c.Log.WithError(err).Warnf("Failed to DM with cancel information")
			}
		}

		p.oauthBroker.UnsubscribeOAuthComplete(c.UserID, ch)
	}()

	http.Redirect(w, r, url, http.StatusFound)
}

func (p *Plugin) completeConnectUserToGoogle(c *Context, w http.ResponseWriter, r *http.Request) {
	var rErr error
	defer func() {
		p.oauthBroker.publishOAuthComplete(c.UserID, rErr, false)
	}()

	config := p.getConfiguration()

	code := r.URL.Query().Get("code")
	// Length looks to be 73 consistently but we'll check for empty and > 100 just in case because this code is generated by Google.
	if len(code) == 0 || len(code) > 100 {
		rErr = errors.New("invalid authorization code")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if len(state) == 0 {
		rErr = errors.New("missing state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	stateSlice := strings.Split(state, "_")
	if len(stateSlice) != 2 {
		rErr = errors.New("invalid state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	userID := stateSlice[1]

	ok := mattermostModel.IsValidId(userID)
	if !ok {
		rErr = errors.New("invalid user id")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	// First part of the state string is 15 characters long and there is also an underscore in the middle.
	validLength := stateRandomStringLength + 1 + len(userID)
	if len(state) != validLength {
		rErr = errors.New("invalid state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	if userID != c.UserID {
		rErr = errors.New("not authorized, incorrect user")
		http.Error(w, rErr.Error(), http.StatusUnauthorized)
		return
	}

	storedState, err := p.KVStore.GetOAuthStateToken(state)
	if err != nil {
		c.Log.WithError(err).Warnf("Can't get state from store")

		rErr = errors.Wrap(err, "missing stored state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	err = p.KVStore.DeleteOAuthStateToken(state)
	if err != nil {
		c.Log.WithError(err).Warnf("Failed to delete state token")

		rErr = errors.Wrap(err, "error deleting stored state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
	}

	if string(storedState) != state {
		rErr = errors.New("invalid state token")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	token, err := p.oauthConfig.Exchange(c.Ctx, code)
	if err != nil {
		c.Log.WithError(err).Warnf("Can't exchange state")

		rErr = errors.Wrap(err, "Failed to exchange OAuth code into token")
		http.Error(w, rErr.Error(), http.StatusInternalServerError)
		return
	}

	jsonToken, err := json.Marshal(token)
	if err != nil {
		c.Log.WithError(err).Warnf("Failed to marshal token to json")
		http.Error(w, errors.Wrap(err, "Failed to marshal token to json").Error(), http.StatusInternalServerError)
		return
	}

	encryptedToken, err := utils.Encrypt([]byte(config.EncryptionKey), jsonToken)
	if err != nil {
		c.Log.WithError(err).Warnf("Failed to encrypt token")
		http.Error(w, errors.Wrap(err, "Failed to encrypt token").Error(), http.StatusInternalServerError)
		return
	}

	if err = p.KVStore.StoreGoogleUserToken(userID, encryptedToken); err != nil {
		c.Log.WithError(err).Warnf("Can't store user token")

		rErr = errors.Wrap(err, "Unable to connect user to Google account")
		http.Error(w, rErr.Error(), http.StatusInternalServerError)
		return
	}

	message := fmt.Sprintf("#### Welcome to the Mattermost Google Drive Plugin!\n"+
		"You've connected your Mattermost account to Google account. Read about the features of this plugin below:\n\n"+
		"##### File Creation\n"+
		"Create Google documents, spreadsheets and presentations with /google-drive create [file type]`.\n\n"+
		"##### Notifications\n"+
		"When someone shares any files with you or comments on any file , you'll get a post here about it.\n\n"+
		"##### File Upload\n"+
		"Check out the Upload to Google Drive button which will allow you to upload message attachments directly to your Google Drive.\n\n"+
		"Click on them!\n\n"+
		"##### Slash Commands\n%s", strings.ReplaceAll(commandHelp, "|", "`"))

	p.createBotDMPost(userID, message, nil)

	p.TrackUserEvent("account_connected", userID, nil)

	p.Client.Frontend.PublishWebSocketEvent(
		"google_connect",
		map[string]any{
			"connected":        true,
			"google_client_id": config.GoogleOAuthClientID,
		},
		&mattermostModel.WebsocketBroadcast{UserId: userID},
	)

	html := `
<!DOCTYPE html>
<html>
	<head>
		<script>
			window.close();
		</script>
	</head>
	<body>
		<p>Completed connecting to Google. Please close this window.</p>
	</body>
</html>
`

	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write([]byte(html)); err != nil {
		p.writeAPIError(w, &APIErrorResponse{ID: "", Message: ">Completed connecting to Google. Please close this window.", StatusCode: http.StatusInternalServerError})
	}
}

func getRawRequestAndFileCreationParams(r *http.Request) (*FileCreationRequest, *mattermostModel.SubmitDialogRequest, error) {
	var request mattermostModel.SubmitDialogRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		return nil, nil, err
	}
	defer deferClose(r.Body)

	submission, err := json.Marshal(request.Submission)
	if err != nil {
		return nil, nil, err
	}
	var fileCreationRequest FileCreationRequest
	err = json.Unmarshal(submission, &fileCreationRequest)
	if err != nil {
		return nil, nil, err
	}

	return &fileCreationRequest, &request, nil
}

func (p *Plugin) handleFileCreation(c *Context, w http.ResponseWriter, r *http.Request) {
	fileCreationParams, request, err := getRawRequestAndFileCreationParams(r)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to get fileCreationParams")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}

	// Verify user is a member of the channel before creating any files
	// This check must happen before file creation to prevent orphaned files
	if request.ChannelId != "" {
		_, appErr := p.API.GetChannelMember(request.ChannelId, c.UserID)
		if appErr != nil {
			c.Log.Warnf("Unauthorized channel access attempt",
				"userID", c.UserID,
				"channelID", request.ChannelId,
				"fileAccess", fileCreationParams.FileAccess)
			p.writeInteractiveDialogError(w, DialogErrorResponse{
				Error:      "You are not a member of the specified channel",
				StatusCode: http.StatusForbidden,
			})
			return
		}
	}

	var fileCreationErr error
	createdFileID := ""
	fileType := r.URL.Query().Get("type")
	if fileType == "" {
		c.Log.Errorf("File type not found in the request")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}
	switch fileType {
	case "doc":
		{
			srv, dErr := p.GoogleClient.NewDocsService(c.Ctx, c.UserID)
			if dErr != nil {
				c.Log.WithError(dErr).Errorf("Failed to create Google Docs client")
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			doc, dErr := srv.Create(c.Ctx, &docs.Document{
				Title: fileCreationParams.Name,
			})
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = doc.DocumentId
		}
	case "slide":
		{
			srv, dErr := p.GoogleClient.NewSlidesService(c.Ctx, c.UserID)
			if dErr != nil {
				c.Log.WithError(dErr).Errorf("Failed to create Google Slides client")
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			slide, dErr := srv.Create(c.Ctx, &slides.Presentation{
				Title: fileCreationParams.Name,
			})
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = slide.PresentationId
		}
	case "sheet":
		{
			srv, dErr := p.GoogleClient.NewSheetsService(c.Ctx, c.UserID)
			if dErr != nil {
				c.Log.WithError(dErr).Errorf("Failed to create Google Sheets client")
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			sheet, dErr := srv.Create(c.Ctx, &sheets.Spreadsheet{
				Properties: &sheets.SpreadsheetProperties{
					Title: fileCreationParams.Name,
				},
			})
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = sheet.SpreadsheetId
		}
	}

	if fileCreationErr != nil {
		c.Log.WithError(fileCreationErr).Errorf("Failed to create Google Drive file")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	err = p.handleFilePermissions(c.Ctx, c.UserID, createdFileID, fileCreationParams.FileAccess, request.ChannelId, fileCreationParams.Name)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to modify file permissions")
		p.writeInteractiveDialogError(w, DialogErrorResponse{Error: "File was successfully created but file permissions failed to apply. Please contact your system administrator.", StatusCode: http.StatusInternalServerError})
		return
	}
	err = p.sendFileCreatedMessage(c.Ctx, request.ChannelId, createdFileID, c.UserID, fileCreationParams.Message, fileCreationParams.ShareInChannel)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to send file creation post")
		p.writeInteractiveDialogError(w, DialogErrorResponse{Error: "File was successfully created but failed to share to the channel. Please contact your system administrator.", StatusCode: http.StatusInternalServerError})
		return
	}
}

func (p *Plugin) handleDriveWatchNotifications(c *Context, w http.ResponseWriter, r *http.Request) {
	resourceState := r.Header.Get("X-Goog-Resource-State")
	userID := r.URL.Query().Get("userID")

	if resourceState != "change" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Validate userID before we do anything.
	_, err := p.Client.User.Get(userID)
	if err != nil {
		p.API.LogError("Failed to get user", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	watchChannelData, err := p.KVStore.GetWatchChannelData(userID)
	if err != nil {
		p.API.LogError("Unable to find watch channel data", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("X-Goog-Channel-Token")
	if watchChannelData.Token == "" || watchChannelData.Token != token {
		p.API.LogError("Invalid channel token", "userID", userID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clusterService := pluginapi.NewClusterService(p.API)
	// Mutex to prevent race conditions from multiple requests directed at the same user in a short period of time.
	m, err := clusterService.NewMutex("drive_watch_notifications_" + userID)
	if err != nil {
		p.API.LogError("Failed to create mutex", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lockErr := m.LockWithContext(c.Ctx)
	if lockErr != nil {
		p.API.LogError("Failed to lock mutex", "err", lockErr, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer m.Unlock()

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, userID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Get the pageToken from the KV store, it has changed since we acquired the lock.
	watchChannelData, err = p.KVStore.GetWatchChannelData(userID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pageToken := watchChannelData.PageToken
	if pageToken == "" {
		// This is to catch any edge cases where the pageToken is not set.
		tokenResponse, tokenErr := driveService.GetStartPageToken(c.Ctx)
		if tokenErr != nil {
			p.API.LogError("Failed to get start page token", "err", tokenErr, "userID", userID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		pageToken = tokenResponse.StartPageToken
	}

	var pageTokenErr error
	var changes []*drive.Change
	for range 5 {
		changeList, changeErr := driveService.ChangesList(c.Ctx, pageToken)
		if changeErr != nil {
			p.API.LogError("Failed to fetch Google Drive changes", "err", changeErr, "userID", userID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		changes = append(changes, changeList.Changes...)
		// NewStartPageToken will be empty if there is another page of results. This should only happen if this user changed over 20/30 files at once. There is no definitive number of changes that will be returned.
		if changeList.NewStartPageToken != "" {
			// Updated pageToken gets saved at the end along with the new FileLastActivity.
			pageToken = changeList.NewStartPageToken
			break
		}
		pageToken = changeList.NextPageToken
	}

	defer func() {
		// There are instances where we don't want to save the pageToken at the end of the request due to a fatal error where we didn't process any notifications.
		if pageTokenErr == nil {
			watchChannelData.PageToken = pageToken
		}
		err = p.KVStore.StoreWatchChannelData(userID, *watchChannelData)
		if err != nil {
			p.API.LogError("Database error occurred while trying to save watch channel data", "err", err, "userID", userID)
			return
		}
	}()

	if len(changes) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	activitySrv, err := p.GoogleClient.NewDriveActivityService(c.Ctx, userID)
	if err != nil {
		pageTokenErr = err
		p.API.LogError("Failed to fetch Google Drive changes", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, change := range changes {
		if change.File == nil {
			continue
		}

		modifiedTime, err := time.Parse(time.RFC3339, change.File.ModifiedTime)
		if err != nil {
			p.API.LogError("Failed to parse modified time", "err", err, "userID", userID, "modifiedTime", change.File.ModifiedTime)
			continue
		}
		lastChangeTime, err := time.Parse(time.RFC3339, change.Time)
		if err != nil {
			p.API.LogError("Failed to parse last change time", "err", err, "userID", userID, "lastChangeTime", change.Time)
			continue
		}
		if change.File.ViewedByMeTime != "" {
			var viewedByMeTime time.Time
			viewedByMeTime, err = time.Parse(time.RFC3339, change.File.ViewedByMeTime)
			if err != nil {
				p.API.LogError("Failed to parse viewed by me time", "err", err, "userID", userID, "viewedByMeTime", change.File.ViewedByMeTime)
				continue
			}

			// Check if the user has already opened the file after the last change.
			if lastChangeTime.Sub(modifiedTime) >= lastChangeTime.Sub(viewedByMeTime) {
				err = p.KVStore.StoreLastActivityForFile(userID, change.FileId, change.File.ViewedByMeTime)
				if err != nil {
					p.API.LogError("Failed to store last activity for file", "err", err, "fileID", change.FileId, "userID", userID)
				}
				continue
			}
		}

		driveActivityQuery := &driveactivity.QueryDriveActivityRequest{
			ItemName: fmt.Sprintf("items/%s", url.PathEscape(change.FileId)),
		}

		lastActivityTime, err := p.KVStore.GetLastActivityForFile(userID, change.FileId)
		if err != nil {
			p.API.LogDebug("Failed to fetch last activity for file", "err", err, "fileID", change.FileId, "userID", userID)
			continue
		}

		if change.File.ViewedByMeTime > lastActivityTime {
			lastActivityTime = change.File.ViewedByMeTime
		}

		// If we have a last activity timestamp for this file we can use it to filter the activities.
		if lastActivityTime != "" {
			driveActivityQuery.Filter = "time > \"" + lastActivityTime + "\""
		} else {
			// PageSize documentation: https://developers.google.com/drive/activity/v2/reference/rest/v2/activity/query#QueryDriveActivityRequest.
			// TLDR: PageSize does not return the exact number of activities that you specify. LastActivity is not set so lets just get the latest activity.
			driveActivityQuery.PageSize = 1
		}

		var activities []*driveactivity.DriveActivity
		for range 5 {
			var activityRes *driveactivity.QueryDriveActivityResponse
			activityRes, err = activitySrv.Query(c.Ctx, driveActivityQuery)
			if err != nil {
				p.API.LogError("Failed to fetch google drive activity", "err", err, "fileID", change.FileId, "userID", userID)
				continue
			}
			for _, activity := range activityRes.Activities {
				if activity.PrimaryActionDetail.Comment != nil || activity.PrimaryActionDetail.PermissionChange != nil {
					if len(activity.Actors) > 0 && activity.Actors[0].User != nil && activity.Actors[0].User.KnownUser != nil && activity.Actors[0].User.KnownUser.IsCurrentUser {
						continue
					}
					activities = append(activities, activity)
				}
			}
			// NextPageToken is set when there are more than 1 page of activities for a file. We don't want the next page token if we are only fetching the latest activity.
			if (activityRes.NextPageToken != "" && driveActivityQuery.PageSize != 1) || (activityRes.NextPageToken != "" && len(activities) <= 5) {
				driveActivityQuery.PageToken = activityRes.NextPageToken
			} else {
				break
			}
		}

		if len(activities) == 0 {
			continue
		}

		// We don't want to spam the user with notifications if there are more than 5 activities.
		if len(activities) > 5 {
			p.handleMultipleActivitiesNotification(change.File, userID)
			lastActivityTime = change.File.ModifiedTime
		} else {
			// Newest activity is at the end of the list so iterate through the list in reverse.
			for i := len(activities) - 1; i >= 0; i-- {
				activity := activities[i]
				if activity.PrimaryActionDetail.Comment != nil {
					lastActivityTime = activity.Timestamp
					p.handleCommentNotifications(c.Ctx, driveService, change.File, userID, activity)
				}

				if activity.PrimaryActionDetail.PermissionChange != nil {
					lastActivityTime = activity.Timestamp
					p.handleFileSharedNotification(change.File, userID)
				}
			}
		}

		err = p.KVStore.StoreLastActivityForFile(userID, change.FileId, lastActivityTime)
		if err != nil {
			p.API.LogError("Failed to store last activity for file", "err", err, "fileID", change.FileId, "userID", userID)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (p *Plugin) openCommentReplyDialog(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		p.API.LogError("Failed to read request body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer deferClose(r.Body)
	var request mattermostModel.PostActionIntegrationRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	commentID, ok := request.Context["commentID"].(string)
	if !ok {
		p.API.LogError("Comment ID not found in the request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fileID, ok := request.Context["fileID"].(string)
	if !ok {
		p.API.LogError("File ID not found in the request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	urlStr := fmt.Sprintf("%s/plugins/%s/api/v1/reply?fileID=%s&commentID=%s",
		*p.API.GetConfig().ServiceSettings.SiteURL,
		url.PathEscape(Manifest.Id),
		url.QueryEscape(fileID),
		url.QueryEscape(commentID))
	dialog := mattermostModel.OpenDialogRequest{
		TriggerId: request.TriggerId,
		URL:       urlStr,
		Dialog: mattermostModel.Dialog{
			CallbackId:     "reply",
			Title:          "Reply to comment",
			Elements:       []mattermostModel.DialogElement{},
			SubmitLabel:    "Reply",
			NotifyOnCancel: false,
			State:          request.PostId,
		},
	}

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, mattermostModel.DialogElement{
		DisplayName: "Message",
		Name:        "message",
		Type:        "textarea",
	})

	appErr := p.API.OpenInteractiveDialog(dialog)
	if appErr != nil {
		p.API.LogWarn("Failed to open interactive dialog", "err", appErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (p *Plugin) handleCommentReplyDialog(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		p.API.LogError("Failed to read request body", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	defer deferClose(r.Body)

	var request mattermostModel.SubmitDialogRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	commentID := r.URL.Query().Get("commentID")
	fileID := r.URL.Query().Get("fileID")

	message, ok := request.Submission["message"].(string)
	if !ok {
		p.API.LogError("Message not found in the request")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", c.UserID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	reply, err := driveService.CreateReply(c.Ctx, fileID, commentID, &drive.Reply{
		Content: message,
	})
	if err != nil {
		p.API.LogError("Failed to create comment reply", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	post := mattermostModel.Post{
		Message:   fmt.Sprintf("You replied to this comment with: \n> %s", reply.Content),
		ChannelId: request.ChannelId,
		RootId:    request.State,
		UserId:    p.BotUserID,
	}
	_, appErr := p.API.CreatePost(&post)
	if appErr != nil {
		p.API.LogWarn("Failed to create post", "err", appErr, "channelID", post.ChannelId, "rootId", post.RootId, "message", post.Message)
		p.writeInteractiveDialogError(w, DialogErrorResponse{Error: "Comment created but failed to create post. Please contact your system administrator", StatusCode: http.StatusInternalServerError})
		return
	}
}

func (p *Plugin) handleFileUpload(c *Context, w http.ResponseWriter, r *http.Request) {
	var request mattermostModel.SubmitDialogRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		p.API.LogError("Failed to decode SubmitDialogRequest", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}
	defer deferClose(r.Body)

	fileID, ok := request.Submission["fileID"].(string)
	if !ok || fileID == "" {
		c.Log.Errorf("File ID not found in the request")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}

	fileInfo, appErr := p.API.GetFileInfo(fileID)
	if appErr != nil {
		c.Log.WithError(appErr).Errorf("Unable to fetch file info", "fileID", fileID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	fileReader, appErr := p.API.GetFile(fileID)
	if appErr != nil {
		c.Log.WithError(appErr).Errorf("Unable to fetch file data", "fileID", fileID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to create Google Drive service")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	_, err = driveService.CreateFile(c.Ctx, &drive.File{
		Name: fileInfo.Name,
	}, fileReader)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to upload file")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	p.API.SendEphemeralPost(c.UserID, &mattermostModel.Post{
		Message:   "Successfully uploaded file in Google Drive.",
		ChannelId: request.ChannelId,
		UserId:    p.BotUserID,
	})
}

func (p *Plugin) handleAllFilesUpload(c *Context, w http.ResponseWriter, r *http.Request) {
	var request mattermostModel.SubmitDialogRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to decode SubmitDialogRequest")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}
	defer deferClose(r.Body)

	postID := request.State
	post, appErr := p.API.GetPost(postID)
	if appErr != nil {
		c.Log.WithError(appErr).Errorf("Failed to get post", "postID", postID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		c.Log.WithError(err).Errorf("Failed to create Google Drive service")
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	fileIDs := post.FileIds
	for _, fileID := range fileIDs {
		fileInfo, appErr := p.API.GetFileInfo(fileID)
		if appErr != nil {
			c.Log.WithError(appErr).Errorf("Unable to get file info", "fileID", fileID)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}

		fileReader, appErr := p.API.GetFile(fileID)
		if appErr != nil {
			c.Log.WithError(appErr).Errorf("Unable to get file", "fileID", fileID)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}

		_, err = driveService.CreateFile(c.Ctx, &drive.File{
			Name: fileInfo.Name,
		}, fileReader)
		if err != nil {
			c.Log.WithError(err).Errorf("Failed to upload file")
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}
	}
	p.API.SendEphemeralPost(c.UserID, &mattermostModel.Post{
		Message:   "Successfully uploaded all files in Google Drive.",
		ChannelId: request.ChannelId,
		UserId:    p.BotUserID,
	})
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) initializeAPI() {
	p.router = mux.NewRouter()
	p.router.Use(p.withRecovery)

	oauthRouter := p.router.PathPrefix("/oauth").Subrouter()
	apiRouter := p.router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(p.checkConfigured)

	oauthRouter.HandleFunc("/connect", p.checkAuth(p.attachContext(p.connectUserToGoogle), ResponseTypePlain)).Methods(http.MethodGet)
	oauthRouter.HandleFunc("/complete", p.checkAuth(p.attachContext(p.completeConnectUserToGoogle), ResponseTypePlain)).Methods(http.MethodGet)

	apiRouter.HandleFunc("/create", p.checkAuth(p.attachContext(p.handleFileCreation), ResponseTypeJSON)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/webhook", p.attachContext(p.handleDriveWatchNotifications)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/reply_dialog", p.checkAuth(p.attachContext(p.openCommentReplyDialog), ResponseTypeJSON)).Methods(http.MethodPost)
	apiRouter.HandleFunc("/reply", p.checkAuth(p.attachContext(p.handleCommentReplyDialog), ResponseTypeJSON)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/upload_file", p.checkAuth(p.attachContext(p.handleFileUpload), ResponseTypeJSON)).Methods(http.MethodPost)
	apiRouter.HandleFunc("/upload_all", p.checkAuth(p.attachContext(p.handleAllFilesUpload), ResponseTypeJSON)).Methods(http.MethodPost)
}
