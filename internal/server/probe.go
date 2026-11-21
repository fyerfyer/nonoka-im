package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ProbeServer is a small Kratos transport.Server for liveness, readiness and
// metrics endpoints. It is used by job applications that have no HTTP/gRPC
// service transport.
type ProbeServer struct {
	name    string
	addr    string
	handler http.Handler
	log     *log.Helper

	listener net.Listener
	server   *http.Server
	ready    func(context.Context) error
}

// NewProbeServer creates a probe server. metrics may be nil; ready may be nil
// (then /readyz always succeeds).
func NewProbeServer(name, addr string, metrics http.Handler, ready func(context.Context) error, logger log.Logger) *ProbeServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	return &ProbeServer{name: name, addr: addr, handler: mux, log: log.NewHelper(logger), ready: ready}
}

func (s *ProbeServer) Start(ctx context.Context) error {
	if s.addr == "" {
		<-ctx.Done()
		return nil
	}
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s probe %s: %w", s.name, s.addr, err)
	}
	s.listener = lis
	s.server = &http.Server{Handler: s.handler, ReadHeaderTimeout: 5 * time.Second}
	s.log.Infof("%s probe server listening on %s", s.name, lis.Addr())
	err = s.server.Serve(lis)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *ProbeServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
