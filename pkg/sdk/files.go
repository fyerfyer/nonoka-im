package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

type FilesResource struct {
	baseURL   string
	client    *http.Client
	tokens    TokenSource
	userAgent string
}

// Upload sends a multipart file and returns the stored attachment metadata.
func (r *FilesResource) Upload(ctx context.Context, name string, content []byte) (*UploadedFile, error) {
	if name == "" {
		name = "unnamed"
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/files", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	if token, tokenErr := r.token(ctx); tokenErr != nil {
		return nil, tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, decodeAPIError(resp)
	}
	var out UploadedFile
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.URL != "" && !strings.HasPrefix(out.URL, "http") {
		out.URL = r.baseURL + out.URL
	}
	return &out, nil
}

// Download fetches an attachment after server-side authorization.
func (r *FilesResource) Download(ctx context.Context, fileID string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/v1/files/"+fileID, nil)
	if err != nil {
		return nil, "", err
	}
	if token, tokenErr := r.token(ctx); tokenErr != nil {
		return nil, "", tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, "", decodeAPIError(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (r *FilesResource) Share(ctx context.Context, fileID string, userIDs []int64, enabled bool) error {
	payload, _ := json.Marshal(map[string]any{"user_ids": userIDs, "enabled": enabled})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/files/"+fileID+"/share", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token, tokenErr := r.token(ctx); tokenErr != nil {
		return tokenErr
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return &APIError{Code: "API_UNREACHABLE", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp)
	}
	return nil
}

func (r *FilesResource) token(ctx context.Context) (string, error) {
	if r.tokens == nil {
		return "", nil
	}
	return r.tokens.Token(ctx)
}
