package rest

import "context"

type MockClient struct {
	createBundle func(ctx context.Context, node string, ID string) (*Bundle, error)
	status       func(ctx context.Context, node string, ID string) (*Bundle, error)
	getFile      func(ctx context.Context, node string, ID string, path string) (err error)
	list         func(ctx context.Context, node string) ([]*Bundle, error)
	delete       func(ctx context.Context, node string, id string) error
}

func (_m *MockClient) CreateBundle(ctx context.Context, node string, ID string) (*Bundle, error) {
	return _m.createBundle(ctx, node, ID)
}

func (_m *MockClient) Delete(ctx context.Context, node string, id string) error {
	return _m.delete(ctx, node, id)
}

func (_m *MockClient) GetFile(ctx context.Context, node string, ID string, path string) error {
	return _m.getFile(ctx, node, ID, path)
}

func (_m *MockClient) List(ctx context.Context, node string) ([]*Bundle, error) {
	return _m.list(ctx, node)
}

func (_m *MockClient) Status(ctx context.Context, node string, ID string) (*Bundle, error) {
	return _m.status(ctx, node, ID)
}
