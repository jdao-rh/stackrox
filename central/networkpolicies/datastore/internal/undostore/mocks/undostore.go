// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/stackrox/rox/central/networkpolicies/datastore/internal/undostore (interfaces: UndoStore)

// Package mocks is a generated GoMock package.
package mocks

import (
	gomock "github.com/golang/mock/gomock"
	storage "github.com/stackrox/rox/generated/storage"
	reflect "reflect"
)

// MockUndoStore is a mock of UndoStore interface
type MockUndoStore struct {
	ctrl     *gomock.Controller
	recorder *MockUndoStoreMockRecorder
}

// MockUndoStoreMockRecorder is the mock recorder for MockUndoStore
type MockUndoStoreMockRecorder struct {
	mock *MockUndoStore
}

// NewMockUndoStore creates a new mock instance
func NewMockUndoStore(ctrl *gomock.Controller) *MockUndoStore {
	mock := &MockUndoStore{ctrl: ctrl}
	mock.recorder = &MockUndoStoreMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockUndoStore) EXPECT() *MockUndoStoreMockRecorder {
	return m.recorder
}

// GetUndoRecord mocks base method
func (m *MockUndoStore) GetUndoRecord(arg0 string) (*storage.NetworkPolicyApplicationUndoRecord, bool, error) {
	ret := m.ctrl.Call(m, "GetUndoRecord", arg0)
	ret0, _ := ret[0].(*storage.NetworkPolicyApplicationUndoRecord)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetUndoRecord indicates an expected call of GetUndoRecord
func (mr *MockUndoStoreMockRecorder) GetUndoRecord(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUndoRecord", reflect.TypeOf((*MockUndoStore)(nil).GetUndoRecord), arg0)
}

// UpsertUndoRecord mocks base method
func (m *MockUndoStore) UpsertUndoRecord(arg0 string, arg1 *storage.NetworkPolicyApplicationUndoRecord) error {
	ret := m.ctrl.Call(m, "UpsertUndoRecord", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertUndoRecord indicates an expected call of UpsertUndoRecord
func (mr *MockUndoStoreMockRecorder) UpsertUndoRecord(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertUndoRecord", reflect.TypeOf((*MockUndoStore)(nil).UpsertUndoRecord), arg0, arg1)
}
