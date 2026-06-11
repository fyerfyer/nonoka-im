package sdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// serviceClient is the HTTP client base for all service layers.
type serviceClient struct {
	baseURL string
	httpCli *khttp.Client
}

// authTransport wraps an http.RoundTripper to add JWT Authorization header.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// newServiceClient creates a new HTTP service client with the given base URL, timeout, and optional JWT token.
func newServiceClient(baseURL string, timeout time.Duration, token string) (*serviceClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required for service client")
	}

	opts := []khttp.ClientOption{
		khttp.WithEndpoint(baseURL),
		khttp.WithTimeout(timeout),
	}

	// If token is provided, wrap the HTTP transport to inject Authorization header.
	if token != "" {
		opts = append(opts, khttp.WithTransport(&authTransport{
			base:  http.DefaultTransport,
			token: token,
		}))
	}

	cli, err := khttp.NewClient(context.Background(), opts...)
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
