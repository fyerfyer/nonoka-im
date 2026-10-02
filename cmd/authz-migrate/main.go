package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/topic"

	_ "github.com/jackc/pgx/v5/stdlib"
	fga "github.com/openfga/go-sdk/client"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if len(os.Args) != 2 {
		log.Fatal("usage: authz-migrate model|backfill")
	}
	var err error
	switch os.Args[1] {
	case "model":
		err = writeModel(ctx)
	case "backfill":
		err = backfill(ctx)
	default:
		log.Fatal("usage: authz-migrate model|backfill")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func writeModel(ctx context.Context) error {
	url := os.Getenv("OPENFGA_API_URL")
	if url == "" {
		return fmt.Errorf("OPENFGA_API_URL required")
	}
	storeID := os.Getenv("OPENFGA_STORE_ID")
	client, err := fga.NewSdkClient(&fga.ClientConfiguration{ApiUrl: url, StoreId: storeID})
	if err != nil {
		return err
	}
	if storeID == "" {
		store, err := client.CreateStore(ctx).Body(fga.ClientCreateStoreRequest{Name: "nonoka-im"}).Execute()
		if err != nil {
			return err
		}
		storeID = store.GetId()
	}
	model, err := authz.Model()
	if err != nil {
		return err
	}
	result, err := client.WriteAuthorizationModel(ctx).Options(fga.ClientWriteAuthorizationModelOptions{StoreId: &storeID}).Body(*model).Execute()
	if err != nil {
		return err
	}
	fmt.Printf("OPENFGA_STORE_ID=%s\nOPENFGA_MODEL_ID=%s\n", storeID, result.GetAuthorizationModelId())
	return nil
}

func backfill(ctx context.Context) error {
	a, err := authz.NewOpenFGAFromEnv()
	if err != nil {
		return err
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return fmt.Errorf("POSTGRES_DSN required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	groups, err := db.QueryContext(ctx, "SELECT group_id, owner_id, topic FROM groups ORDER BY group_id")
	if err != nil {
		return err
	}
	groupCount := 0
	for groups.Next() {
		var id, raw string
		var owner int64
		if err := groups.Scan(&id, &owner, &raw); err != nil {
			groups.Close()
			return err
		}
		if err := a.SetGroupOwner(ctx, id, owner); err != nil {
			groups.Close()
			return err
		}
		if err := a.SetTopicGroup(ctx, raw, id); err != nil {
			groups.Close()
			return err
		}
		groupCount++
	}
	if err := groups.Err(); err != nil {
		groups.Close()
		return err
	}
	groups.Close()
	members, err := db.QueryContext(ctx, "SELECT group_id, user_id FROM group_members ORDER BY group_id, user_id")
	if err != nil {
		return err
	}
	memberCount := 0
	for members.Next() {
		var id string
		var uid int64
		if err := members.Scan(&id, &uid); err != nil {
			members.Close()
			return err
		}
		if err := a.SetGroupMember(ctx, id, uid, true); err != nil {
			members.Close()
			return err
		}
		memberCount++
	}
	if err := members.Err(); err != nil {
		members.Close()
		return err
	}
	members.Close()
	topics, err := db.QueryContext(ctx, "SELECT DISTINCT topic FROM conversations ORDER BY topic")
	if err != nil {
		return err
	}
	topicCount := 0
	for topics.Next() {
		var raw string
		if err := topics.Scan(&raw); err != nil {
			topics.Close()
			return err
		}
		switch topic.ParseType(raw) {
		case topic.TypeP2P:
			x, y, err := topic.ExtractUserIDsFromP2PTopic(raw)
			if err != nil {
				topics.Close()
				return err
			}
			if err := a.SetTopicParticipant(ctx, raw, x); err != nil {
				topics.Close()
				return err
			}
			if err := a.SetTopicParticipant(ctx, raw, y); err != nil {
				topics.Close()
				return err
			}
			topicCount++
		case topic.TypeSystem:
			owner, err := strconv.ParseInt(strings.TrimPrefix(raw, topic.PrefixSystem), 10, 64)
			if err != nil {
				topics.Close()
				return err
			}
			if err := a.SetTopicParticipant(ctx, raw, owner); err != nil {
				topics.Close()
				return err
			}
			topicCount++
		}
	}
	if err := topics.Err(); err != nil {
		topics.Close()
		return err
	}
	topics.Close()
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return fmt.Errorf("MONGODB_URI required")
	}
	mongoDB := os.Getenv("MONGODB_DATABASE")
	if mongoDB == "" {
		mongoDB = "nonoka_im"
	}
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())
	files, err := client.Database(mongoDB).Collection("files").Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"owner_id": 1}))
	if err != nil {
		return err
	}
	defer files.Close(ctx)
	fileCount := 0
	for files.Next(ctx) {
		var row struct {
			ID      string `bson:"_id"`
			OwnerID int64  `bson:"owner_id"`
		}
		if err := files.Decode(&row); err != nil {
			return err
		}
		if err := a.SetFileOwner(ctx, row.ID, row.OwnerID); err != nil {
			return err
		}
		if err := a.CanReadFile(ctx, row.OwnerID, row.ID); err != nil {
			return fmt.Errorf("reconcile file %s: %w", row.ID, err)
		}
		fileCount++
	}
	if err := files.Err(); err != nil {
		return err
	}
	fmt.Printf("reconciled groups=%d members=%d topics=%d files=%d\n", groupCount, memberCount, topicCount, fileCount)
	return nil
}
