package integration

import (
	"testing"
	"time"

	agentdata "nonoka-im/internal/data/agent"
	"nonoka-im/pkg/sdk"
)

func newTestSDKClient(t *testing.T, token, deviceID string) *sdk.Client {
	t.Helper()
	realtime := sdk.DefaultRealtimeConfig()
	realtime.GatewayURLs = []string{testWSURL}
	realtime.DeviceID = deviceID
	realtime.HeartbeatInterval = 5 * time.Second
	client, err := sdk.New(sdk.Config{
		BaseURL: testBaseURL, TokenSource: sdk.StaticToken(token),
		Timeout: 15 * time.Second, Realtime: realtime,
	})
	if err != nil {
		t.Fatalf("new sdk client: %v", err)
	}
	return client
}

func newAgentStore(ts *testServer) *agentdata.Store {
	return agentdata.NewStore(ts.data.DB())
}
