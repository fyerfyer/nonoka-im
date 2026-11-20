package messaging

import (
	"context"
	"fmt"

	"nonoka-im/internal/filestore"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// File is the read model of one attachment.
type File struct {
	Name string
	Mime string
	Data []byte
}

// Files reads attachments from MongoDB; authorization stays with the caller.
type Files struct{ db *mongo.Database }

func NewFiles(db *mongo.Database) *Files { return &Files{db: db} }

func (f *Files) ReadFile(ctx context.Context, fileID string) (File, error) {
	if f == nil || f.db == nil {
		return File{}, fmt.Errorf("file reader unavailable")
	}
	doc, err := filestore.Read(ctx, f.db, fileID)
	if err != nil {
		return File{}, err
	}
	return File{Name: doc.Name, Mime: doc.Mime, Data: doc.Data}, nil
}
