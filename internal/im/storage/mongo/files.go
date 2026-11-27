package mongostore

import (
	"context"
	"errors"
	"time"

	"nonoka-im/internal/im/app"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CollectionFiles stores message attachments as single documents.
const CollectionFiles = "im_files"

type fileRow struct {
	ID        string    `bson:"_id"`
	Name      string    `bson:"name"`
	Mime      string    `bson:"mime"`
	Size      int64     `bson:"size"`
	OwnerID   int64     `bson:"owner_id"`
	Data      []byte    `bson:"data"`
	CreatedAt time.Time `bson:"created_at"`
}

// FileStore persists attachments in MongoDB.
type FileStore struct{ db *mongo.Database }

func NewFileStore(db *mongo.Database) *FileStore { return &FileStore{db: db} }

func (s *FileStore) Put(ctx context.Context, file app.FileObject) error {
	_, err := s.db.Collection(CollectionFiles).InsertOne(ctx, fileRow{
		ID: file.ID, Name: file.Name, Mime: file.Mime, Size: file.Size,
		OwnerID: file.OwnerID, Data: file.Data, CreatedAt: file.CreatedAt,
	})
	return err
}

func (s *FileStore) Get(ctx context.Context, fileID string) (app.FileObject, error) {
	var row fileRow
	err := s.db.Collection(CollectionFiles).FindOne(ctx, bson.M{"_id": fileID}).Decode(&row)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.FileObject{}, app.ErrFileNotFound
		}
		return app.FileObject{}, err
	}
	return app.FileObject{
		ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size,
		OwnerID: row.OwnerID, Data: row.Data, CreatedAt: row.CreatedAt,
	}, nil
}

// GetMeta loads attachment metadata without the file body. Sharing and
// authorization checks must not pull a large attachment into memory.
func (s *FileStore) GetMeta(ctx context.Context, fileID string) (app.FileObject, error) {
	var row fileRow
	err := s.db.Collection(CollectionFiles).FindOne(ctx, bson.M{"_id": fileID},
		options.FindOne().SetProjection(bson.M{"data": 0})).Decode(&row)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.FileObject{}, app.ErrFileNotFound
		}
		return app.FileObject{}, err
	}
	return app.FileObject{
		ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size,
		OwnerID: row.OwnerID, CreatedAt: row.CreatedAt,
	}, nil
}

var _ app.FileStore = (*FileStore)(nil)
