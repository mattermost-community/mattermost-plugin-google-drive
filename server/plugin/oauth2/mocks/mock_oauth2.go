// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2 (interfaces: Config)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	oauth2 "golang.org/x/oauth2"
)

// MockConfigInterface is a mock of Config interface.
type MockConfigInterface struct {
	ctrl     *gomock.Controller
	recorder *MockConfigInterfaceMockRecorder
}

// MockConfigInterfaceMockRecorder is the mock recorder for MockConfigInterface.
type MockConfigInterfaceMockRecorder struct {
	mock *MockConfigInterface
}

// NewMockConfigInterface creates a new mock instance.
func NewMockConfigInterface(ctrl *gomock.Controller) *MockConfigInterface {
	mock := &MockConfigInterface{ctrl: ctrl}
	mock.recorder = &MockConfigInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfigInterface) EXPECT() *MockConfigInterfaceMockRecorder {
	return m.recorder
}

// AuthCodeURL mocks base method.
func (m *MockConfigInterface) AuthCodeURL(arg0 string) string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AuthCodeURL", arg0)
	ret0, _ := ret[0].(string)
	return ret0
}

// AuthCodeURL indicates an expected call of AuthCodeURL.
func (mr *MockConfigInterfaceMockRecorder) AuthCodeURL(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AuthCodeURL", reflect.TypeOf((*MockConfigInterface)(nil).AuthCodeURL), arg0)
}

// Exchange mocks base method.
func (m *MockConfigInterface) Exchange(arg0 context.Context, arg1 string) (*oauth2.Token, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Exchange", arg0, arg1)
	ret0, _ := ret[0].(*oauth2.Token)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Exchange indicates an expected call of Exchange.
func (mr *MockConfigInterfaceMockRecorder) Exchange(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Exchange", reflect.TypeOf((*MockConfigInterface)(nil).Exchange), arg0, arg1)
}

// TokenSource mocks base method.
func (m *MockConfigInterface) TokenSource(arg0 context.Context, arg1 *oauth2.Token) oauth2.TokenSource {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TokenSource", arg0, arg1)
	ret0, _ := ret[0].(oauth2.TokenSource)
	return ret0
}

// TokenSource indicates an expected call of TokenSource.
func (mr *MockConfigInterfaceMockRecorder) TokenSource(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TokenSource", reflect.TypeOf((*MockConfigInterface)(nil).TokenSource), arg0, arg1)
}
