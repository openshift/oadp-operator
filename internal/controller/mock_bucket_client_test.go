package controller

import (
	"fmt"

	bucketpkg "github.com/openshift/oadp-operator/pkg/bucket"
)

// mockBucketClient is a mock implementation of bucketpkg.Client for testing
type mockBucketClient struct {
	// Control behavior
	existsResult    bool
	existsError     error
	createResult    bool
	createError     error
	deleteResult    bool
	deleteError     error
	getResult       string
	getError        error
	reconcileResult bool
	reconcileError  error

	// Track calls
	existsCalled    int
	createCalled    int
	deleteCalled    int
	getCalled       int
	reconcileCalled int
}

// Ensure mockBucketClient implements bucketpkg.Client
var _ bucketpkg.Client = &mockBucketClient{}

func (m *mockBucketClient) Exists() (bool, error) {
	m.existsCalled++
	return m.existsResult, m.existsError
}

func (m *mockBucketClient) Create() (bool, error) {
	m.createCalled++
	return m.createResult, m.createError
}

func (m *mockBucketClient) Delete() (bool, error) {
	m.deleteCalled++
	return m.deleteResult, m.deleteError
}

func (m *mockBucketClient) Get(_ string) (string, error) {
	m.getCalled++
	if m.getError != nil {
		return "", m.getError
	}
	return m.getResult, nil
}

func (m *mockBucketClient) Reconcile() (bool, error) {
	m.reconcileCalled++
	return m.reconcileResult, m.reconcileError
}

// Helper function to create a mock that simulates permission denied error
func newPermissionDeniedMock() *mockBucketClient {
	return &mockBucketClient{
		existsResult: false,
		existsError:  nil,
		createResult: false,
		createError:  fmt.Errorf("403 Forbidden: Permission denied"),
	}
}

// Helper function to create a mock that simulates successful bucket creation
func newSuccessfulMock() *mockBucketClient {
	return &mockBucketClient{
		existsResult: false,
		existsError:  nil,
		createResult: true,
		createError:  nil,
	}
}

// Helper function to create a mock that simulates bucket already exists
func newAlreadyExistsMock() *mockBucketClient {
	return &mockBucketClient{
		existsResult: true,
		existsError:  nil,
	}
}
