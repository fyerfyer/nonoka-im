// Package filestore defines the shared MongoDB layout for message
// attachments. Both the HTTP file service and the agent read_file tool read
// through this package so they agree on the document shape and collection.
package filestore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// CollectionFiles stores message attachments as single documents.
// Note: mongo-driver v2 dropped the GridFS package, so attachments are kept
// as one BSON document per file. The default MaxUploadBytes (10MB) stays
// under BSON's 16MB per-document limit, so no chunking is needed.
const CollectionFiles = "im_files"

// File is one attachment document in CollectionFiles.
type File struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	Mime      string    `bson:"mime"`
	Size      int64     `bson:"size"`
	OwnerID   int64     `bson:"owner_id"`
	Data      []byte    `bson:"data"`
	CreatedAt time.Time `bson:"created_at"`
}

var ErrNotFound = errors.New("file not found")

// Read loads one attachment by ID.
func Read(ctx context.Context, db *mongo.Database, id string) (*File, error) {
	var doc File
	err := db.Collection(CollectionFiles).FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}
