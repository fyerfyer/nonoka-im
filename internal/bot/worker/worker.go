package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	botapp "nonoka-im/internal/bot/app"
	botdata "nonoka-im/internal/data/bot"
	"nonoka-im/internal/kafka"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
)

// Worker runs the webhook delivery loop and consumes message trigger events.
type Worker struct {
	Consumer  *kafka.Consumer
	Delivery  *botapp.DeliveryService
	Store     *botdata.Store
	Metrics   *metrics.Metrics
	Log       *log.Helper
	Workers   int
	Poll      time.Duration
	StopGrace time.Duration
	Retention time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func (w *Worker) log() *log.Helper {
	if w.Log != nil {
		return w.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

// Start launches delivery workers and blocks on the Kafka consumer.
func (w *Worker) Start(ctx context.Context) error {
	if w.Consumer == nil {
		return fmt.Errorf("kafka consumer is required")
	}
	if w.Delivery == nil {
		return fmt.Errorf("delivery service is required")
	}
	if w.Workers <= 0 {
		w.Workers = 8
	}
	if w.Poll <= 0 {
		w.Poll = 200 * time.Millisecond
	}
	if w.StopGrace <= 0 {
		w.StopGrace = 30 * time.Second
	}
	if w.Retention <= 0 {
		w.Retention = 30 * 24 * time.Hour
	}
	w.stopCh = make(chan struct{})
	for i := 0; i < w.Workers; i++ {
		w.wg.Add(1)
		go w.deliveryLoop(ctx, i)
	}
	w.wg.Add(1)
	go w.maintenanceLoop(ctx)
	return w.Consumer.Start(ctx)
}

func (w *Worker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		if w.stopCh != nil {
			close(w.stopCh)
		}
	})
	var err error
	if w.Consumer != nil {
		err = w.Consumer.Stop()
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(w.StopGrace):
		w.log().Warnf("bot worker shutdown timed out after %s", w.StopGrace)
	}
	return err
}

// HandleEvent implements kafka.MessageHandler.
func (w *Worker) HandleEvent(ctx context.Context, key, value []byte, _ map[string]string) error {
	var event messaging.MessageEvent
	if err := json.Unmarshal(value, &event); err != nil {
		return fmt.Errorf("unmarshal bot trigger event: %w", err)
	}
	return w.Delivery.Dispatch(ctx, event)
}

func (w *Worker) deliveryLoop(ctx context.Context, id int) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}
		worked, err := w.Delivery.DeliverNext(ctx)
		if err != nil {
			w.log().Warnf("bot delivery worker %d error: %v", id, err)
			if !sleep(ctx, w.Poll) {
				return
			}
			continue
		}
		if !worked {
			if !sleep(ctx, w.Poll) {
				return
			}
		}
	}
}

func (w *Worker) maintenanceLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if n, err := w.Store.ResetStaleDeliveries(ctx, 2*time.Minute); err != nil {
				w.log().Warnf("reset stale bot deliveries: %v", err)
			} else if n > 0 {
				w.log().Infof("reset %d stale bot deliveries", n)
			}
			cutoff := time.Now().Add(-w.Retention)
			if n, err := w.Store.CleanupDeliveries(ctx, cutoff, 5000); err != nil {
				w.log().Warnf("cleanup bot deliveries: %v", err)
			} else if n > 0 {
				w.log().Infof("cleaned up %d old bot deliveries", n)
			}
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
