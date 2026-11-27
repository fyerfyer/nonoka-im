package sdk

import (
	"context"
	"net/http"
	"net/url"

	"nonoka-im/pkg/sdk/internal/transport"

	"google.golang.org/protobuf/proto"
)

// TokenSource supplies access tokens. Implementations should be safe for
// concurrent use and refresh proactively.
type TokenSource = transport.TokenSource

// RefreshableTokenSource can rotate an expired access token without user
// interaction. HTTP transports retry once after a successful refresh.
type RefreshableTokenSource = transport.RefreshableTokenSource

// APIError is the typed error returned for HTTP API failures.
type APIError = transport.APIError

// jsonInt64 is kept as an internal alias for token response decoding.
type jsonInt64 = transport.JSONInt64

// apiTransport is a thin SDK-facing wrapper around the single internal HTTP
// transport. Keeping the wrapper lets resources use short method names while
// the retry/refresh/error pipeline lives in one place.
type apiTransport struct {
	*transport.Client
}

func newAPITransport(baseURL string, client *http.Client, tokens TokenSource, userAgent string) *apiTransport {
	return &apiTransport{Client: transport.New(baseURL, client, tokens, userAgent)}
}

func (t *apiTransport) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any, retry bool) error {
	return t.Client.DoJSON(ctx, method, path, query, body, out, retry)
}

func (t *apiTransport) doProto(ctx context.Context, method, path string, query url.Values, body, out proto.Message) error {
	return t.Client.DoProto(ctx, method, path, query, body, out)
}

func (t *apiTransport) doRequest(req *http.Request) (*http.Response, error) {
	return t.Client.Do(req)
}

func decodeAPIError(resp *http.Response) error {
	return transport.DecodeAPIError(resp)
}
