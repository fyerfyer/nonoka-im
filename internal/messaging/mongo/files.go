// Package mongostore contains messaging adapters backed by the IM file store.
package mongostore

import (
	"context"

	"nonoka-im/internal/im/app"
	"nonoka-im/internal/messaging"
)

// Files reads attachments through the IM file usecase.
type Files struct{ Store app.FileStore }

func NewFiles(store app.FileStore) *Files { return &Files{Store: store} }

func (f *Files) ReadFile(ctx context.Context, fileID string) (messaging.File, error) {
	doc, err := f.Store.Get(ctx, fileID)
	if err != nil {
		return messaging.File{}, err
	}
	return messaging.File{Name: doc.Name, Mime: doc.Mime, Data: doc.Data}, nil
}
