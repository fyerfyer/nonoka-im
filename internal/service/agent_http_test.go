package service

import "testing"

func TestValidateModelBaseURLRejectsSSRFTargets(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "public https", raw: "https://example.com/v1", wantErr: false},
		{name: "loopback literal", raw: "https://127.0.0.1/v1", wantErr: true},
		{name: "loopback ipv6", raw: "https://[::1]/v1", wantErr: true},
		{name: "metadata", raw: "https://169.254.169.254/latest", wantErr: true},
		{name: "private", raw: "https://10.0.0.5/v1", wantErr: true},
		{name: "localhost name", raw: "https://localhost/v1", wantErr: true},
		{name: "plain http", raw: "http://example.com/v1", wantErr: true},
		{name: "bad scheme", raw: "ftp://example.com/v1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateModelBaseURL(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateModelBaseURL(%q) err=%v, wantErr=%v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

func TestValidateAgentRequestUsesToolCatalog(t *testing.T) {
	v := &agentRequest{Name: "catalog-agent", Instruction: "be helpful", Tools: []string{"send_message", "get_group_members"}}
	if err := validateAgentRequest(v); err != nil {
		t.Fatalf("catalog tools rejected: %v", err)
	}
	v.Tools = []string{"not_a_real_tool"}
	if err := validateAgentRequest(v); err == nil {
		t.Fatal("unknown tool accepted; validation is still hardcoded")
	}
}
