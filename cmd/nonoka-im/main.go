package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/llm"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

	_ "go.uber.org/automaxprocs"
)

func platformAdminIDs() []int64 {
	raw := strings.TrimSpace(os.Getenv("AGENT_ADMIN_USER_IDS"))
	if raw == "" {
		return nil
	}
	out := make([]int64, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name = "nonoka-im"
	// Version is the version of the compiled software.
	Version = "dev"
	// flagconf is the config flag.
	flagconf string
	// nodeID is the unique gateway node ID. Defaults to hostname.
	nodeID string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.StringVar(&nodeID, "node-id", "", "gateway node id, defaults to hostname")
}

func newApp(logger log.Logger, gs *grpc.Server, hs *http.Server, registry *gateway.GatewayRegistry, peerBus *gateway.PeerBus, ws *gateway.WebSocketServer, hb gateway.HeartbeatConfig, models *llm.Service) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
		kratos.BeforeStart(func(ctx context.Context) error {
			if models != nil {
				if err := models.EnsurePlatformAdmins(ctx, platformAdminIDs()); err != nil {
					log.NewHelper(logger).Warnf("bootstrap platform admins: %v", err)
				}
			}
			if registry != nil {
				registry.StartHeartbeat()
			}
			if peerBus != nil {
				if err := peerBus.Start(ctx); err != nil {
					return err
				}
			}
			if ws != nil {
				idleTimeout := hb.Timeout
				if idleTimeout <= 0 {
					idleTimeout = 90 * time.Second
				}
				interval := hb.Interval
				if interval <= 0 {
					interval = 30 * time.Second
				}
				ws.StartIdleChecker(idleTimeout, interval)
			}
			return nil
		}),
		kratos.BeforeStop(func(ctx context.Context) error {
			if registry != nil {
				registry.Stop(ctx)
			}
			if peerBus != nil {
				peerBus.Stop()
			}
			return nil
		}),
	)
}

func main() {
	flag.Parse()
	if nodeID != "" {
		id = nodeID
	}
	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, bc.Auth, bc.Dispatch, bc.Gateway, bc.Search, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
