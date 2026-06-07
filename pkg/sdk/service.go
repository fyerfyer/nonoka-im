package sdk

import (
	"context"
	"fmt"
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// serviceClient is the HTTP client base for all service layers.
type serviceClient struct {
	baseURL string
	httpCli *khttp.Client
}

// newServiceClient creates a new HTTP service client with the given base URL and timeout.
func newServiceClient(baseURL string, timeout time.Duration) (*serviceClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required for service client")
	}

	cli, err := khttp.NewClient(
		context.Background(),
		khttp.WithEndpoint(baseURL),
		khttp.WithTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create kratos http client: %w", err)
	}

	return &serviceClient{
		baseURL: baseURL,
		httpCli: cli,
	}, nil
}

// close releases the underlying HTTP client resources.
func (c *serviceClient) close() error {
	if c.httpCli != nil {
		return c.httpCli.Close()
	}
	return nil
}
