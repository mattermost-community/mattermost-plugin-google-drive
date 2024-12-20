// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2 (interfaces: Config)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	config "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
	oauth2 "golang.org/x/oauth2"
)

// MockConfig is a mock of Config interface.
type MockConfig struct {
	ctrl     *gomock.Controller
	recorder *MockConfigMockRecorder
}

// MockConfigMockRecorder is the mock recorder for MockConfig.
type MockConfigMockRecorder struct {
	mock *MockConfig
}

// NewMockConfig creates a new mock instance.
func NewMockConfig(ctrl *gomock.Controller) *MockConfig {
	mock := &MockConfig{ctrl: ctrl}
	mock.recorder = &MockConfigMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfig) EXPECT() *MockConfigMockRecorder {
	return m.recorder
}

// AuthCodeURL mocks base method.
func (m *MockConfig) AuthCodeURL(arg0 string) string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AuthCodeURL", arg0)
	ret0, _ := ret[0].(string)
	return ret0
}

// AuthCodeURL indicates an expected call of AuthCodeURL.
func (mr *MockConfigMockRecorder) AuthCodeURL(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AuthCodeURL", reflect.TypeOf((*MockConfig)(nil).AuthCodeURL), arg0)
}

// Exchange mocks base method.
func (m *MockConfig) Exchange(arg0 context.Context, arg1 string) (*oauth2.Token, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Exchange", arg0, arg1)
	ret0, _ := ret[0].(*oauth2.Token)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Exchange indicates an expected call of Exchange.
func (mr *MockConfigMockRecorder) Exchange(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Exchange", reflect.TypeOf((*MockConfig)(nil).Exchange), arg0, arg1)
}

// ReloadConfig mocks base method.
func (m *MockConfig) ReloadConfig(arg0 *config.Configuration) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReloadConfig", arg0)
}

// ReloadConfig indicates an expected call of ReloadConfig.
func (mr *MockConfigMockRecorder) ReloadConfig(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReloadConfig", reflect.TypeOf((*MockConfig)(nil).ReloadConfig), arg0)
}

// TokenSource mocks base method.
func (m *MockConfig) TokenSource(arg0 context.Context, arg1 *oauth2.Token) oauth2.TokenSource {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TokenSource", arg0, arg1)
	ret0, _ := ret[0].(oauth2.TokenSource)
	return ret0
}

// TokenSource indicates an expected call of TokenSource.
func (mr *MockConfigMockRecorder) TokenSource(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TokenSource", reflect.TypeOf((*MockConfig)(nil).TokenSource), arg0, arg1)
}
