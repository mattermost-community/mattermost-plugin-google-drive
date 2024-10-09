package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
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
				p.client.Log.Warn("Recovered from a panic",
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
		p.client.Log.Warn("Can't write api error http response", "err", err.Error())
	}
}

func (p *Plugin) writeInteractiveDialogError(w http.ResponseWriter, errResponse DialogErrorResponse) {
	w.WriteHeader(errResponse.StatusCode)
	if errResponse.Error == "" {
		errResponse.Error = "Something went wrong, please contact your system administrator"
	}
	if err := json.NewEncoder(w).Encode(errResponse); err != nil {
		p.client.Log.Warn("Can't write api error http response", "err", err.Error())
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
				p.client.Log.Debug("Unknown ResponseType detected")
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

	state := fmt.Sprintf("%v_%v", model.NewId()[0:15], userID)

	if _, err := p.client.KV.Set(state, []byte(state)); err != nil {
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
			_, err := p.poster.DMWithAttachments(userID, &model.SlackAttachment{
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

	var storedState []byte
	err := p.client.KV.Get(state, &storedState)
	if err != nil {
		c.Log.WithError(err).Warnf("Can't get state from store")

		rErr = errors.Wrap(err, "missing stored state")
		http.Error(w, rErr.Error(), http.StatusBadRequest)
		return
	}

	err = p.client.KV.Delete(state)
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

		rErr = errors.Wrap(err, "Failed to exchange oauth code into token")
		http.Error(w, rErr.Error(), http.StatusInternalServerError)
		return
	}

	if err = p.storeGoogleUserToken(userID, token); err != nil {
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

	p.client.Frontend.PublishWebSocketEvent(
		"google_connect",
		map[string]interface{}{
			"connected":        true,
			"google_client_id": config.GoogleOAuthClientID,
		},
		&model.WebsocketBroadcast{UserId: userID},
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

func getRawRequestAndFileCreationParams(r *http.Request) (*FileCreationRequest, *model.SubmitDialogRequest, error) {
	var request model.SubmitDialogRequest
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

	ctx := context.Background()
	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(request.UserId)
	if err != nil {
		p.API.LogError("Failed to get google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	var fileCreationErr error
	createdFileID := ""
	fileType := r.URL.Query().Get("type")
	switch fileType {
	case "doc":
		{
			srv, dErr := docs.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create google docs client", "err", dErr)
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
			srv, dErr := slides.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create google slides client", "err", dErr)
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
			srv, dErr := sheets.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
			if dErr != nil {
				p.API.LogError("Failed to create google sheets client", "err", dErr)
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
		p.API.LogError("Failed to create google drive file", "err", fileCreationErr)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	err = p.handleFilePermissions(request.UserId, createdFileID, fileCreationParams.FileAccess, request.ChannelId)
	if err != nil {
		p.API.LogError("Failed to modify file permissions", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	err = p.sendFileCreatedMessage(request.ChannelId, createdFileID, request.UserId, fileCreationParams.Message, fileCreationParams.ShareInChannel, authToken)
	if err != nil {
		p.API.LogError("Failed to send file creation post", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
}

func (p *Plugin) handleDriveWatchNotifications(c *Context, w http.ResponseWriter, r *http.Request) {
	var watchChannelData WatchChannelData
	resourceState := r.Header.Get("X-Goog-Resource-State")
	userID := r.URL.Query().Get("userID")

	if resourceState != "change" {
		w.WriteHeader(http.StatusOK)
		return
	}

	err := p.client.KV.Get(getWatchChannelDataKey(userID), &watchChannelData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token := r.Header.Get("X-Goog-Channel-Token")
	if watchChannelData.Token != token {
		p.API.LogError("Invalid channel token")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(userID)
	if err != nil {
		p.API.LogError("Failed to get google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	srv, err := drive.NewService(context.Background(), option.WithTokenSource(conf.TokenSource(context.Background(), authToken)))
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Mutex to prevent multiple requests from the same user.
	m, err := cluster.NewMutex(p.API, "drive_watch_notifications_"+userID)
	if err != nil {
		p.API.LogError("Failed to create mutex", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lockErr := m.LockWithContext(c.Ctx)
	if lockErr != nil {
		p.API.LogError("Failed to lock mutex", "err", lockErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer m.Unlock()

	// Get the pageToken from the KV store, it has changed since we acquired the lock.
	err = p.client.KV.Get(getWatchChannelDataKey(userID), &watchChannelData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pageToken := watchChannelData.PageToken
	if pageToken == "" {
		tokenResponse, tokenErr := srv.Changes.GetStartPageToken().Do()
		if tokenErr != nil {
			p.API.LogError("Failed to get start page token", "err", tokenErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		pageToken = tokenResponse.StartPageToken
	}

	var pageTokenErr error
	var changes []*drive.Change
	for {
		changeList, changeErr := srv.Changes.List(pageToken).Fields("*").Do()
		if changeErr != nil {
			p.API.LogError("Failed to fetch Google Drive changes", "err", changeErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		changes = append(changes, changeList.Changes...)
		// NewStartPageToken will be empty if there is another page of results. This should only happen if this user changed over 20/30 files at once. There is no definitive number.
		if changeList.NewStartPageToken != "" {
			// Updated pageToken gets saved at the end along with the new FileLastActivity.
			pageToken = changeList.NewStartPageToken
			p.API.LogDebug("Page tokens", "NewStartPageToken", changeList.NewStartPageToken, "pageToken", pageToken, "lengthOfChangeList.Changes", len(changeList.Changes))
			break
		}
		p.API.LogDebug("NextPageToken populated", "NextPageToken", changeList.NextPageToken)
		pageToken = changeList.NextPageToken
	}

	defer func() {
		// There are instances where we don't want to save the pageToken at the end of the request due to a fatal error.
		if pageTokenErr == nil {
			watchChannelData.PageToken = pageToken
		}
		saved, kvErr := p.client.KV.Set(getWatchChannelDataKey(userID), watchChannelData)
		if !saved && kvErr != nil {
			p.API.LogError("Database error occureed while trying to save watch channel data", "err", kvErr)
			w.WriteHeader(http.StatusBadRequest)
			return
		} else if !saved && kvErr == nil {
			p.API.LogError("Failed to save watch channel data")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}()

	if len(changes) == 0 {
		p.API.LogInfo("No Google Drive changes found", "pageToken", pageToken)
		w.WriteHeader(http.StatusOK)
		return
	}

	activitySrv, err := driveactivity.NewService(context.Background(), option.WithTokenSource(conf.TokenSource(context.Background(), authToken)))
	if err != nil {
		pageTokenErr = err
		p.API.LogError("Failed to fetch google drive changes", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, change := range changes {
		if change.File == nil {
			p.API.LogDebug("No file found", "pageToken", pageToken)
			continue
		}

		modifiedTime, _ := time.Parse(time.RFC3339, change.File.ModifiedTime)
		lastChangeTime, _ := time.Parse(time.RFC3339, change.Time)
		viewedByMeTime, _ := time.Parse(time.RFC3339, change.File.ViewedByMeTime)

		if lastChangeTime.Sub(modifiedTime) > lastChangeTime.Sub(viewedByMeTime) {
			p.API.LogDebug("User has already opened the file after the change.")
			continue
		}

		driveActivityQuery := &driveactivity.QueryDriveActivityRequest{
			ItemName: fmt.Sprintf("items/%s", change.FileId),
		}

		lastActivity := watchChannelData.FileLastActivity[change.FileId]
		p.API.LogDebug("Last activity", "lastActivity", lastActivity, "fileID", change.FileId)
		// If we have a last activity timestamp for this file we can use it to filter the activities.
		if lastActivity != "" {
			driveActivityQuery.Filter = "time > \"" + lastActivity + "\""
		} else {
			// PageSize documentation: https://developers.google.com/drive/activity/v2/reference/rest/v2/activity/query#QueryDriveActivityRequest.
			// TLDR: PageSize does not return the exact number of activities that you specify. LastActivity is not set so lets just get the latest activity.
			driveActivityQuery.PageSize = 1
		}

		activityRes, err := activitySrv.Activity.Query(driveActivityQuery).Do()
		if err != nil {
			p.API.LogDebug("Failed to fetch google drive activity", "err", err, "fileID", change.FileId)
			continue
		}

		if activityRes == nil || activityRes.Activities == nil || len(activityRes.Activities) == 0 {
			p.API.LogDebug("No activities found for File", "fileID", change.FileId)
			continue
		}

		// Newest activity is at the end of the list so iterate through the list in reverse.
		for i := len(activityRes.Activities) - 1; i >= 0; i-- {
			activity := activityRes.Activities[i]
			if activity.PrimaryActionDetail.Comment != nil {
				if activity.Timestamp > lastActivity {
					lastActivity = activity.Timestamp
				}
				if len(activity.Actors) > 0 && activity.Actors[0].User != nil && activity.Actors[0].User.KnownUser != nil && activity.Actors[0].User.KnownUser.IsCurrentUser {
					continue
				}
				p.handleCommentNotifications(change.FileId, userID, activity, authToken)
			}
			if activity.PrimaryActionDetail.PermissionChange != nil {
				if activity.Timestamp > lastActivity {
					lastActivity = activity.Timestamp
				}
				if len(activity.Actors) > 0 && activity.Actors[0].User != nil && activity.Actors[0].User.KnownUser != nil && activity.Actors[0].User.KnownUser.IsCurrentUser {
					continue
				}
				p.handleFileSharedNotification(change.FileId, userID, authToken)
			}
		}

		if lastActivity > watchChannelData.FileLastActivity[change.FileId] {
			watchChannelData.FileLastActivity[change.FileId] = lastActivity
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
	var request model.PostActionIntegrationRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	commentID := request.Context["commentID"].(string)
	fileID := request.Context["fileID"].(string)
	dialog := model.OpenDialogRequest{
		TriggerId: request.TriggerId,
		URL:       fmt.Sprintf("%s/plugins/%s/api/v1/reply?fileID=%s&commentID=%s", *p.API.GetConfig().ServiceSettings.SiteURL, manifest.Id, fileID, commentID),
		Dialog: model.Dialog{
			CallbackId:     "reply",
			Title:          "Reply to comment",
			Elements:       []model.DialogElement{},
			SubmitLabel:    "Reply",
			NotifyOnCancel: false,
			State:          request.PostId,
		},
	}

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
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

	var request model.SubmitDialogRequest
	err = json.Unmarshal(requestData, &request)
	if err != nil {
		p.API.LogError("Failed to parse request body", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	commentID := r.URL.Query().Get("commentID")
	fileID := r.URL.Query().Get("fileID")

	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(request.UserId)
	if err != nil {
		p.API.LogError("Failed to get google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	srv, err := drive.NewService(context.Background(), option.WithTokenSource(conf.TokenSource(context.Background(), authToken)))
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}
	reply, err := srv.Replies.Create(fileID, commentID, &drive.Reply{
		Content: request.Submission["message"].(string),
	}).Fields("*").Do()
	if err != nil {
		p.API.LogError("Failed to create comment reply", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	post := model.Post{
		Message:   fmt.Sprintf("You replied to this comment with: \n> %s", reply.Content),
		ChannelId: request.ChannelId,
		RootId:    request.State,
		UserId:    p.BotUserID,
	}
	_, appErr := p.API.CreatePost(&post)
	if appErr != nil {
		p.API.LogWarn("Failed to create post", "err", appErr, "channelID", post.ChannelId, "rootId", post.RootId, "message", post.Message)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (p *Plugin) handleFileUpload(c *Context, w http.ResponseWriter, r *http.Request) {
	var request model.SubmitDialogRequest
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

	ctx := context.Background()
	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(c.UserID)
	if err != nil {
		p.API.LogError("Failed to get google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	srv, err := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	_, err = srv.Files.Create(&drive.File{
		Name: fileInfo.Name,
	}).Media(bytes.NewReader(fileReader)).Do()
	if err != nil {
		p.API.LogError("Failed to upload file", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	p.API.SendEphemeralPost(c.UserID, &model.Post{
		Message:   "Successfully uploaded file in Google Drive.",
		ChannelId: request.ChannelId,
		UserId:    p.BotUserID,
	})
}

func (p *Plugin) handleAllFilesUpload(c *Context, w http.ResponseWriter, r *http.Request) {
	var request model.SubmitDialogRequest
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

	ctx := context.Background()
	conf := p.getOAuthConfig()

	authToken, err := p.getGoogleUserToken(c.UserID)
	if err != nil {
		p.API.LogError("Failed to get google user token", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	srv, err := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err)
		p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
		return
	}

	fileIds := post.FileIds
	for _, fileID := range fileIds {
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

		_, err := srv.Files.Create(&drive.File{
			Name: fileInfo.Name,
		}).Media(bytes.NewReader(fileReader)).Do()
		if err != nil {
			p.API.LogError("Failed to upload file", "err", err)
			p.writeInteractiveDialogError(w, DialogErrorResponse{StatusCode: http.StatusInternalServerError})
			return
		}
	}
	p.API.SendEphemeralPost(c.UserID, &model.Post{
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

	apiRouter.HandleFunc("/create", p.attachContext(p.handleFileCreation)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/webhook", p.attachContext(p.handleDriveWatchNotifications)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/reply_dialog", p.attachContext(p.openCommentReplyDialog)).Methods(http.MethodPost)
	apiRouter.HandleFunc("/reply", p.attachContext(p.handleCommentReplyDialog)).Methods(http.MethodPost)

	apiRouter.HandleFunc("/upload_file", p.attachContext(p.handleFileUpload)).Methods(http.MethodPost)
	apiRouter.HandleFunc("/upload_all", p.attachContext(p.handleAllFilesUpload)).Methods(http.MethodPost)
}
