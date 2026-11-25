// Command search-index rebuilds the optional Meilisearch message index from
// MongoDB. It is idempotent and safe to run while the stack is online.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"nonoka-im/internal/conf"
	"nonoka-im/internal/im/search"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	var flagconf string
	var batch int
	flag.StringVar(&flagconf, "conf", "../../configs", "config path")
	flag.IntVar(&batch, "batch", 500, "documents per Meilisearch request")
	flag.Parse()

	c := config.New(config.WithSource(file.NewSource(flagconf)))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}
	if bc.Data == nil || bc.Data.Mongodb == nil {
		panic("mongodb config is required")
	}
	client := search.FromConfig(bc.Search)
	if !client.Enabled() {
		fmt.Fprintln(os.Stderr, "MEILISEARCH_URL / search.url is not configured")
		os.Exit(2)
	}
	if err := client.EnsureIndex(context.Background()); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(bc.Data.Mongodb.Uri))
	if err != nil {
		panic(err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	db := mongoClient.Database(bc.Data.Mongodb.Database)
	n, err := search.Backfill(ctx, db, client, batch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("indexed %d documents into %s\n", n, client.Index)
}
