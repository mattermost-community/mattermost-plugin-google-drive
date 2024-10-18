package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gorilla/mux"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/flow"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

// ResponseType indicates type of response returned by api
type ResponseType string

const (
	// ResponseTypeJSON indicates that response type is json
	ResponseTypeJSON ResponseType = "JSON_RESPONSE"
	// ResponseTypePlain indicates that response type is text plain
	ResponseTypePlain ResponseType = "TEXT_RESPONSE"

	APIErrorIDNotConnected = "not_connected"
	requestTimeout         = 60 * time.Second
)

type Context struct {
	Ctx    context.Context
	UserID string
	Log    logger.Logger
}

type UserContext struct {
	Context
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

// This is an error response used in interactive dialogs
type DialogErrorResponse struct {
	Error      string `json:"error"`
	StatusCode int    `json:"status_code"`
}

// HTTPHandlerFuncWithUserContext is http.HandleFunc but with a UserContext attached
type HTTPHandlerFuncWithUserContext func(c *UserContext, w http.ResponseWriter, r *http.Request)

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
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	conf := p.getOAuthConfig()

	state := fmt.Sprintf("%v_%v", mattermostModel.NewId()[0:15], userID)

	if err := p.KVStore.StoreOAuthStateToken(state, state); err != nil {
		c.Log.WithError(err).Warnf("Can't store state oauth2")
		http.Error(w, "can't store state oauth2", http.StatusInternalServerError)
		return
	}

	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))

	ch := p.oauthBroker.SubscribeOAuthComplete(userID)

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
			_, err := p.poster.DMWithAttachments(userID, &mattermostModel.SlackAttachment{
				Text:  fmt.Sprintf("There was an error connecting to your Google account: `%s` Please double check your configuration.", errorMsg),
				Color: string(flow.ColorDanger),
			})
			if err != nil {
				c.Log.WithError(err).Warnf("Failed to DM with cancel information")
			}
		}

		p.oauthBroker.UnsubscribeOAuthComplete(userID, ch)
	}()

	http.Redirect(w, r, url, http.StatusFound)
}

func (p *Plugin) completeConnectUserToGoogle(c *Context, w http.ResponseWriter, r *http.Request) {
	authedUserID := r.Header.Get("Mattermost-User-ID")
	if authedUserID == "" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	var rErr error
	defer func() {
		p.oauthBroker.publishOAuthComplete(authedUserID, rErr, false)
	}()

	config := p.getConfiguration()

	conf := p.getOAuthConfig()

	code := r.URL.Query().Get("code")
	if len(code) == 0 {
		rErr = errors.New("missing authorization code")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")

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

	userID := strings.Split(state, "_")[1]

	if userID != authedUserID {
		rErr = errors.New("not authorized, incorrect user")
		http.Error(w, rErr.Error(), http.StatusUnauthorized)
		return
	}

	token, err := conf.Exchange(c.Ctx, code)
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

	encryptedToken, err := utils.Encrypt([]byte(config.EncryptionKey), string(jsonToken))
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
		map[string]interface{}{
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
	defer r.Body.Close()

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
		p.API.LogError("Failed to get fileCreationParams", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}

	driveService, driveServiceError := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if driveServiceError != nil {
		p.API.LogError("Failed to create Google Drive client", "err", driveServiceError)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	for i := 0; i < 11; i++ {
		_, aboutErr := driveService.About(c.Ctx, "*")
		if aboutErr != nil {
			p.API.LogError("Failed to get Google Drive about", "err", aboutErr)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}
	}

	if driveService != nil {
		return
	}
	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(c.UserID)
	if err != nil {
		p.API.LogError("Failed to get Google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	var fileCreationErr error
	createdFileID := ""
	fileType := r.URL.Query().Get("type")
	switch fileType {
	case "doc":
		{
			srv, dErr := docs.NewService(c.Ctx, option.WithTokenSource(conf.TokenSource(c.Ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create Google Docs client", "err", dErr)
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			doc, dErr := srv.Documents.Create(&docs.Document{
				Title: fileCreationParams.Name,
			}).Do()
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = doc.DocumentId
		}
	case "slide":
		{
			srv, dErr := slides.NewService(c.Ctx, option.WithTokenSource(conf.TokenSource(c.Ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create Google Slides client", "err", dErr)
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			slide, dErr := srv.Presentations.Create(&slides.Presentation{
				Title: fileCreationParams.Name,
			}).Do()
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = slide.PresentationId
		}
	case "sheet":
		{
			srv, dErr := sheets.NewService(c.Ctx, option.WithTokenSource(conf.TokenSource(c.Ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create Google Sheets client", "err", dErr)
				p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
				return
			}
			sheet, dErr := srv.Spreadsheets.Create(&sheets.Spreadsheet{
				Properties: &sheets.SpreadsheetProperties{
					Title: fileCreationParams.Name,
				},
			}).Do()
			if dErr != nil {
				fileCreationErr = dErr
				break
			}
			createdFileID = sheet.SpreadsheetId
		}
	}

	if fileCreationErr != nil {
		p.API.LogError("Failed to create Google Drive file", "err", fileCreationErr)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	err = p.handleFilePermissions(c.Ctx, c.UserID, createdFileID, fileCreationParams.FileAccess, request.ChannelId, fileCreationParams.Name)
	if err != nil {
		p.API.LogError("Failed to modify file permissions", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{Error: "File was successfully created but file permissions failed to apply. Please contact your system administrator.", StatusCode: http.StatusInternalServerError})
		return
	}
	err = p.sendFileCreatedMessage(c.Ctx, request.ChannelId, createdFileID, c.UserID, fileCreationParams.Message, fileCreationParams.ShareInChannel)
	if err != nil {
		p.API.LogError("Failed to send file creation post", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{Error: "File was successfully created but failed to share to the channel. Please contact your system administrator.", StatusCode: http.StatusInternalServerError})
		return
	}
}

func (p *Plugin) handleDriveWatchNotifications(c *Context, w http.ResponseWriter, r *http.Request) {
	resourceState := r.Header.Get("X-Goog-Resource-State")
	userID := r.URL.Query().Get("userID")

	_, _ = p.Client.KV.Set("userID-"+userID, userID, pluginapi.SetExpiry(20))
	if resourceState != "change" {
		w.WriteHeader(http.StatusOK)
		return
	}

	watchChannelData, err := p.KVStore.GetWatchChannelData(userID)
	if err != nil {
		p.API.LogError("Unable to fund watch channel data", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("X-Goog-Channel-Token")
	if watchChannelData.Token != token {
		p.API.LogError("Invalid channel token", "userID", userID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(userID)
	if err != nil {
		p.API.LogError("Failed to get Google user token", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, userID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", userID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Mutex to prevent race conditions from multiple requests directed at the same user in a short period of time.
	m, err := cluster.NewMutex(p.API, "drive_watch_notifications_"+userID)
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
	for {
		changeList, changeErr := driveService.ChangesList(c.Ctx, pageToken)
		if changeErr != nil {
			p.API.LogError("Failed to fetch Google Drive changes", "err", changeErr, "userID", userID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		changes = append(changes, changeList.Changes...)
		// NewStartPageToken will be empty if there is another page of results. This should only happen if this user changed over 20/30 files at once. There is no definitive number.
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
			p.API.LogError("Database error occureed while trying to save watch channel data", "err", err, "userID", userID)
			return
		}
	}()

	if len(changes) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	activitySrv, err := driveactivity.NewService(context.Background(), option.WithTokenSource(conf.TokenSource(context.Background(), authToken)))
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

		modifiedTime, _ := time.Parse(time.RFC3339, change.File.ModifiedTime)
		lastChangeTime, _ := time.Parse(time.RFC3339, change.Time)
		viewedByMeTime, _ := time.Parse(time.RFC3339, change.File.ViewedByMeTime)

		// Check if the user has already opened the file after the last change.
		if lastChangeTime.Sub(modifiedTime) > lastChangeTime.Sub(viewedByMeTime) {
			continue
		}

		driveActivityQuery := &driveactivity.QueryDriveActivityRequest{
			ItemName: fmt.Sprintf("items/%s", change.FileId),
		}

		lastActivityTime, err := p.KVStore.GetLastActivityForFile(userID, change.FileId)
		if err != nil {
			p.API.LogDebug("Failed to fetch last activity for file", "err", err, "fileID", change.FileId, "userID", userID)
			continue
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
		for {
			var activityRes *driveactivity.QueryDriveActivityResponse
			activityRes, err = activitySrv.Activity.Query(driveActivityQuery).Do()
			if err != nil {
				p.API.LogError("Failed to fetch google drive activity", "err", err, "fileID", change.FileId, "userID", userID)
				continue
			}
			activities = append(activities, activityRes.Activities...)
			// NextPageToken is set when there are more than 1 page of activities for a file. We don't want the next page token if we are only fetching the latest activity.
			if activityRes.NextPageToken != "" && driveActivityQuery.PageSize != 1 {
				driveActivityQuery.PageToken = activityRes.NextPageToken
			} else {
				break
			}
		}

		if len(activities) == 0 {
			continue
		}
		newLastActivityTime := lastActivityTime
		// Newest activity is at the end of the list so iterate through the list in reverse.
		for i := len(activities) - 1; i >= 0; i-- {
			activity := activities[i]
			if activity.PrimaryActionDetail.Comment != nil {
				if activity.Timestamp > lastActivityTime {
					newLastActivityTime = activity.Timestamp
				}
				if len(activity.Actors) > 0 && activity.Actors[0].User != nil && activity.Actors[0].User.KnownUser != nil && activity.Actors[0].User.KnownUser.IsCurrentUser {
					continue
				}
				p.handleCommentNotifications(c.Ctx, driveService, change.File, userID, activity)
			}
			if activity.PrimaryActionDetail.PermissionChange != nil {
				if activity.Timestamp > lastActivityTime {
					newLastActivityTime = activity.Timestamp
				}
				if len(activity.Actors) > 0 && activity.Actors[0].User != nil && activity.Actors[0].User.KnownUser != nil && activity.Actors[0].User.KnownUser.IsCurrentUser {
					continue
				}
				p.handleFileSharedNotification(change.File, userID)
			}
		}

		if newLastActivityTime > lastActivityTime {
			err = p.KVStore.StoreLastActivityForFile(userID, change.FileId, newLastActivityTime)
			if err != nil {
				p.API.LogError("Failed to store last activity for file", "err", err, "fileID", change.FileId, "userID", userID)
			}
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
	defer r.Body.Close()
	var request mattermostModel.PostActionIntegrationRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	commentID := request.Context["commentID"].(string)
	fileID := request.Context["fileID"].(string)
	dialog := mattermostModel.OpenDialogRequest{
		TriggerId: request.TriggerId,
		URL:       fmt.Sprintf("%s/plugins/%s/api/v1/reply?fileID=%s&commentID=%s", *p.API.GetConfig().ServiceSettings.SiteURL, Manifest.Id, fileID, commentID),
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
	defer r.Body.Close()

	var request mattermostModel.SubmitDialogRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	commentID := r.URL.Query().Get("commentID")
	fileID := r.URL.Query().Get("fileID")

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", c.UserID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	reply, err := driveService.CreateReply(c.Ctx, fileID, commentID, &drive.Reply{
		Content: request.Submission["message"].(string),
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
	defer r.Body.Close()

	fileID := request.Submission["fileID"].(string)
	fileInfo, appErr := p.API.GetFileInfo(fileID)
	if appErr != nil {
		p.API.LogError("Unable to fetch file info", "err", appErr, "fileID", fileID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	fileReader, appErr := p.API.GetFile(fileID)
	if appErr != nil {
		p.API.LogError("Unable to fetch file data", "err", appErr, "fileID", fileID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", c.UserID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	_, err = driveService.CreateFile(c.Ctx, &drive.File{
		Name: fileInfo.Name,
	}, fileReader)
	if err != nil {
		p.API.LogError("Failed to upload file", "err", err)
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
		p.API.LogError("Failed to decode SubmitDialogRequest", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusBadRequest})
		return
	}
	defer r.Body.Close()

	postID := request.State
	post, appErr := p.API.GetPost(postID)
	if appErr != nil {
		p.API.LogError("Failed to get post", "err", appErr, "postID", postID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	driveService, err := p.GoogleClient.NewDriveService(c.Ctx, c.UserID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", c.UserID)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	fileIDs := post.FileIds
	for _, fileID := range fileIDs {
		fileInfo, appErr := p.API.GetFileInfo(fileID)
		if appErr != nil {
			p.API.LogError("Unable to get file info", "err", appErr, "fileID", fileID)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}

		fileReader, appErr := p.API.GetFile(fileID)
		if appErr != nil {
			p.API.LogError("Unable to get file", "err", appErr, "fileID", fileID)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}

		_, err = driveService.CreateFile(c.Ctx, &drive.File{
			Name: fileInfo.Name,
		}, fileReader)
		if err != nil {
			p.API.LogError("Failed to upload file", "err", err)
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
