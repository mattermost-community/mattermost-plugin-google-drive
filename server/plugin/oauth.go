package plugin

import (
	"fmt"
	"net/url"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthCompleteEvent struct {
	UserID string
	Err    error
}

type OAuthBroker struct {
	sendOAuthCompleteEvent func(event OAuthCompleteEvent)

	lock              sync.RWMutex // Protects closed and pingSubs
	closed            bool
	oauthCompleteSubs map[string][]chan error
	mapCreate         sync.Once
}

func (p *Plugin) getOAuthConfig() *oauth2.Config {
	config := p.getConfiguration()

	scopes := []string{
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.activity",
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/presentations",
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/drive",
	}

	redirectURL := fmt.Sprintf("%s/plugins/%s/oauth/complete",
		*p.Client.Configuration.GetConfig().ServiceSettings.SiteURL,
		url.PathEscape(Manifest.Id))

	return &oauth2.Config{
		ClientID:     config.GoogleOAuthClientID,
		ClientSecret: config.GoogleOAuthClientSecret,
		Scopes:       scopes,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
	}
}

func (ob *OAuthBroker) publishOAuthComplete(userID string, err error, fromCluster bool) {
	ob.lock.Lock()
	defer ob.lock.Unlock()

	if ob.closed {
		return
	}

	for _, userSub := range ob.oauthCompleteSubs[userID] {
		// non-blocking send
		select {
		case userSub <- err:
		default:
		}
	}

	if !fromCluster {
		ob.sendOAuthCompleteEvent(OAuthCompleteEvent{UserID: userID, Err: err})
	}
}

func NewOAuthBroker(sendOAuthCompleteEvent func(event OAuthCompleteEvent)) *OAuthBroker {
	return &OAuthBroker{
		sendOAuthCompleteEvent: sendOAuthCompleteEvent,
	}
}

func (ob *OAuthBroker) SubscribeOAuthComplete(userID string) <-chan error {
	ob.lock.Lock()
	defer ob.lock.Unlock()

	ob.mapCreate.Do(func() {
		ob.oauthCompleteSubs = make(map[string][]chan error)
	})

	ch := make(chan error, 1)
	ob.oauthCompleteSubs[userID] = append(ob.oauthCompleteSubs[userID], ch)

	return ch
}

func (ob *OAuthBroker) UnsubscribeOAuthComplete(userID string, ch <-chan error) {
	ob.lock.Lock()
	defer ob.lock.Unlock()

	for i, sub := range ob.oauthCompleteSubs[userID] {
		if sub == ch {
			ob.oauthCompleteSubs[userID] = append(ob.oauthCompleteSubs[userID][:i], ob.oauthCompleteSubs[userID][i+1:]...)
			break
		}
	}
}

func (ob *OAuthBroker) Close() {
	ob.lock.Lock()
	defer ob.lock.Unlock()

	if !ob.closed {
		ob.closed = true

		for _, userSubs := range ob.oauthCompleteSubs {
			for _, sub := range userSubs {
				close(sub)
			}
		}
	}
}
