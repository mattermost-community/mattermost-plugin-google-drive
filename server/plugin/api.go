package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/flow"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
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

type APIErrorResponse struct {
	ID         string `json:"id"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

func (e *APIErrorResponse) Error() string {
	return e.Message
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
		p.client.Log.Warn("can't write api error http response", "err", err.Error())
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

	userId := strings.Split(state, "_")[1]

	if userId != authedUserID {
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

	if err != nil {
		c.Log.WithError(err).Warnf("Can't exchange state")

		rErr = errors.Wrap(err, "Failed to exchange oauth code into token")
		http.Error(w, rErr.Error(), http.StatusInternalServerError)
		return
	}

	if err = p.storeGoogleUserToken(userId, token); err != nil {
		c.Log.WithError(err).Warnf("Can't store user token")

		rErr = errors.Wrap(err, "Unable to connect user to Google account")
		http.Error(w, rErr.Error(), http.StatusInternalServerError)
		return
	}

	message := fmt.Sprintf("#### Welcome to the Mattermost Google Drive Plugin!\n"+
		"You've connected your Mattermost account to Google account. Read about the features of this plugin below:\n\n"+
		"##### File Creation\n"+
		"Create Google documents, spreadsheets and presentations with /gdrive create [file type]`.\n\n"+
		"##### Notifications\n"+
		"When someone shares any files with you or comments on any file , you'll get a post here about it.\n\n"+
		"##### File Upload\n"+
		"Check out the Upload to Google Drive button which will allow you to upload message attachments directly to your Google Drive.\n\n"+
		"Click on them!\n\n"+
		"##### Slash Commands\n%s", strings.ReplaceAll(commandHelp, "|", "`"))

	p.CreateBotDMPost(userId, message, nil)

	p.TrackUserEvent("account_connected", userId, nil)

	p.client.Frontend.PublishWebSocketEvent(
		"google_connect",
		map[string]interface{}{
			"connected":        true,
			"google_client_id": config.GoogleOAuthClientID,
		},
		&model.WebsocketBroadcast{UserId: userId},
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
