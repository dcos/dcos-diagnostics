package rest

import "context"

type AlternativeMockClient struct {
	createBundle func(ctx context.Context, node string, ID string) (*Bundle, error)
	status func(ctx context.Context, node string, ID string) (*Bundle, error)
	getFile func(ctx context.Context, node string, ID string, path string) (err error)
	list func(ctx context.Context, node string) ([]*Bundle, error)
	delete func(ctx context.Context, node string, id string) error
}

func (_m *AlternativeMockClient) CreateBundle(ctx context.Context, node string, ID string) (*Bundle, error) {
	return _m.createBundle(ctx, node, ID)
}

func (_m *AlternativeMockClient) Delete(ctx context.Context, node string, id string) error {
	return _m.delete(ctx, node, id)
}

func (_m *AlternativeMockClient) GetFile(ctx context.Context, node string, ID string, path string) error {
	return _m.getFile(ctx, node, ID, path)
}

func (_m *AlternativeMockClient) List(ctx context.Context, node string) ([]*Bundle, error) {
	return _m.list(ctx, node)
}

func (_m *AlternativeMockClient) Status(ctx context.Context, node string, ID string) (*Bundle, error) {
	return _m.status(ctx, node, ID)
}
