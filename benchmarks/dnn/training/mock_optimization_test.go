// Code generated by MockGen. DO NOT EDIT.
// Source: gitlab.com/akita/dnn/training/optimization (interfaces: Alg)

package training

import (
	gomock "github.com/golang/mock/gomock"
	optimization "gitlab.com/akita/dnn/training/optimization"
	reflect "reflect"
)

// MockAlg is a mock of Alg interface
type MockAlg struct {
	ctrl     *gomock.Controller
	recorder *MockAlgMockRecorder
}

// MockAlgMockRecorder is the mock recorder for MockAlg
type MockAlgMockRecorder struct {
	mock *MockAlg
}

// NewMockAlg creates a new mock instance
func NewMockAlg(ctrl *gomock.Controller) *MockAlg {
	mock := &MockAlg{ctrl: ctrl}
	mock.recorder = &MockAlgMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockAlg) EXPECT() *MockAlgMockRecorder {
	return m.recorder
}

// UpdateParameters mocks base method
func (m *MockAlg) UpdateParameters(arg0 optimization.Layer) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "UpdateParameters", arg0)
}

// UpdateParameters indicates an expected call of UpdateParameters
func (mr *MockAlgMockRecorder) UpdateParameters(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateParameters", reflect.TypeOf((*MockAlg)(nil).UpdateParameters), arg0)
}