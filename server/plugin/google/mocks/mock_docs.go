// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google (interfaces: DocsInterface)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	docs "google.golang.org/api/docs/v1"
)

// MockDocsInterface is a mock of DocsInterface interface.
type MockDocsInterface struct {
	ctrl     *gomock.Controller
	recorder *MockDocsInterfaceMockRecorder
}

// MockDocsInterfaceMockRecorder is the mock recorder for MockDocsInterface.
type MockDocsInterfaceMockRecorder struct {
	mock *MockDocsInterface
}

// NewMockDocsInterface creates a new mock instance.
func NewMockDocsInterface(ctrl *gomock.Controller) *MockDocsInterface {
	mock := &MockDocsInterface{ctrl: ctrl}
	mock.recorder = &MockDocsInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDocsInterface) EXPECT() *MockDocsInterfaceMockRecorder {
	return m.recorder
}

// Create mocks base method.
func (m *MockDocsInterface) Create(arg0 context.Context, arg1 *docs.Document) (*docs.Document, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", arg0, arg1)
	ret0, _ := ret[0].(*docs.Document)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Create indicates an expected call of Create.
func (mr *MockDocsInterfaceMockRecorder) Create(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockDocsInterface)(nil).Create), arg0, arg1)
}
