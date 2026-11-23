package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/authz"

	"github.com/gorilla/websocket"
)

// This file holds the integration coverage for the OpenFGA authorization
// path. The test assembly (setup_test.go) injects *authz.OpenFGA into the
// gateway, message, group, file and agent services, so every assertion below
// exercises the real Check/Write pipeline against the test OpenFGA container.

// wsAuthConnect dials the test gateway and authenticates the given token,
// returning the ready connection.
func wsAuthConnect(t *testing.T, token, deviceID string) *websocket.Conn {
	t.Helper()
	wsConn := wsConnect(t)
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: 1,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{Token: token, DeviceId: deviceID},
		},
	})
	resp := wsReadPacket(t, wsConn, 2*time.Second)
	if errResp := resp.GetError(); errResp != nil {
		wsConn.Close()
		t.Fatalf("ws auth failed: code=%d message=%s", errResp.Code, errResp.Message)
	}
	if resp.GetAuthResp() == nil || !resp.GetAuthResp().Success {
		wsConn.Close()
		t.Fatalf("expected successful auth response, got %+v", resp)
	}
	return wsConn
}

// wsPublish sends one message over the authenticated connection and returns
// the server's response packet (SendReply on success, Error on denial).
func wsPublish(t *testing.T, wsConn *websocket.Conn, seq uint64, topic, clientMsgID string) *v1.Packet {
	t.Helper()
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: seq,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       topic,
				MsgType:     v1.MsgType_MSG_TYPE_TEXT,
				Content:     []byte("authz probe"),
				ClientMsgId: clientMsgID,
			},
		},
	})
	return wsReadPacket(t, wsConn, 3*time.Second)
}

// wsPull sends a pull request over the authenticated connection and returns
// the server's response packet (PullReply on success, Error on denial).
func wsPull(t *testing.T, wsConn *websocket.Conn, seq uint64, topic string, lastSeq uint64) *v1.Packet {
	t.Helper()
	wsSendPacket(t, wsConn, &v1.Packet{
		Cmd: v1.Command_CMD_PULL,
		Seq: seq,
		Payload: &v1.Packet_PullReq{
			PullReq: &v1.PullRequest{Topic: topic, LastSeq: lastSeq, Limit: 10},
		},
	})
	return wsReadPacket(t, wsConn, 3*time.Second)
}

// createGroupHTTP creates a group through the public HTTP API (owner = token's
// user, plus the given initial members) and returns its ID and topic. Going
// through the service ensures both PostgreSQL membership and the OpenFGA
// tuples are written exactly like a real client would.
func createGroupHTTP(t *testing.T, token, name string, memberIDs ...int64) (string, string) {
	t.Helper()
	resp := httpPostAuth(t, testBaseURL+"/v1/groups", token, map[string]any{
		"name":      name,
		"memberIds": memberIDs,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create group failed: status %d: %s", resp.StatusCode, body)
	}
	var reply v1.CreateGroupReply
	decodeProtoJSON(t, resp.Body, &reply)
	if reply.GroupId == "" || reply.Topic == "" {
		t.Fatalf("expected group_id and topic in reply, got %+v", &reply)
	}
	return reply.GroupId, reply.Topic
}

// TestAuthz_P2PFirstSendEstablishesParticipants verifies that the first
// publish to a P2P topic writes participant tuples for both users into
// OpenFGA: the peer can pull immediately and an unrelated user is rejected.
func TestAuthz_P2PFirstSendEstablishesParticipants(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenA, userA := registerAndLogin(t, "authz-p2p-a", "123456")
	tokenB, userB := registerAndLogin(t, "authz-p2p-b", "123456")
	tokenC, userC := registerAndLogin(t, "authz-p2p-c", "123456")

	lo, hi := min(userA, userB), max(userA, userB)
	topic := fmt.Sprintf("p2p_%d_%d", lo, hi)

	// A publishes the first message; the ACK must succeed and establish
	// participants for both sides of the conversation.
	wsA := wsAuthConnect(t, tokenA, "authz-p2p-a-dev")
	defer wsA.Close()
	ack := wsPublish(t, wsA, 2, topic, "authz-p2p-first")
	if ack.GetSendReply() == nil {
		t.Fatalf("expected publish ACK, got %+v", ack)
	}

	// Direct assertions on the authorization state.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, uid := range []int64{userA, userB} {
		if err := ts.openFGA.CanAccessTopic(ctx, uid, topic); err != nil {
			t.Fatalf("expected participant %d to access %s, got %v", uid, topic, err)
		}
	}
	if err := ts.openFGA.CanAccessTopic(ctx, userC, topic); !errors.Is(err, authz.ErrDenied) {
		t.Fatalf("expected unrelated user %d to be denied on %s, got %v", userC, topic, err)
	}

	// Behavioral: the peer pulls successfully right after the first send.
	wsB := wsAuthConnect(t, tokenB, "authz-p2p-b-dev")
	defer wsB.Close()
	pullB := wsPull(t, wsB, 2, topic, 0)
	if pullB.GetPullReply() == nil {
		t.Fatalf("expected peer pull to succeed, got %+v", pullB)
	}

	// Behavioral: an unrelated user pulling the same topic is rejected 4030.
	wsC := wsAuthConnect(t, tokenC, "authz-p2p-c-dev")
	defer wsC.Close()
	pullC := wsPull(t, wsC, 2, topic, 0)
	errC := pullC.GetError()
	if errC == nil {
		t.Fatalf("expected outsider pull to be rejected, got %+v", pullC)
	}
	if errC.Code != 4030 {
		t.Fatalf("expected error code 4030, got %d (%s)", errC.Code, errC.Message)
	}

	t.Logf("p2p participants established on first send: topic=%s", topic)
}

// TestAuthz_GroupNonMemberDenied verifies that group topics are backed by
// OpenFGA group tuples: members can publish, non-members are rejected with
// 4030 on both publish and pull, and the group API itself refuses them.
func TestAuthz_GroupNonMemberDenied(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, userOwner := registerAndLogin(t, "authz-grp-owner", "123456")
	tokenMember, userMember := registerAndLogin(t, "authz-grp-member", "123456")
	tokenOutsider, _ := registerAndLogin(t, "authz-grp-outsider", "123456")

	groupID, topic := createGroupHTTP(t, tokenOwner, "authz-group", userOwner, userMember)

	// Positive control: a member publishes successfully.
	wsMember := wsAuthConnect(t, tokenMember, "authz-grp-member-dev")
	defer wsMember.Close()
	ack := wsPublish(t, wsMember, 2, topic, "authz-grp-member-msg")
	if ack.GetSendReply() == nil {
		t.Fatalf("expected member publish ACK, got %+v", ack)
	}

	// Non-member publish is rejected with 4030.
	wsOutsider := wsAuthConnect(t, tokenOutsider, "authz-grp-outsider-dev")
	defer wsOutsider.Close()
	denied := wsPublish(t, wsOutsider, 2, topic, "authz-grp-outsider-msg")
	errPub := denied.GetError()
	if errPub == nil {
		t.Fatalf("expected outsider publish to be rejected, got %+v", denied)
	}
	if errPub.Code != 4030 {
		t.Fatalf("expected publish error code 4030, got %d (%s)", errPub.Code, errPub.Message)
	}

	// Non-member pull over the gateway is rejected with 4030 too.
	deniedPull := wsPull(t, wsOutsider, 3, topic, 0)
	errPull := deniedPull.GetError()
	if errPull == nil {
		t.Fatalf("expected outsider pull to be rejected, got %+v", deniedPull)
	}
	if errPull.Code != 4030 {
		t.Fatalf("expected pull error code 4030, got %d (%s)", errPull.Code, errPull.Message)
	}

	// The HTTP pull fallback enforces the same boundary (kratos 403).
	pullURL := fmt.Sprintf("%s/v1/message/pull?topic=%s&lastSeq=0&limit=10&endSeq=0", testBaseURL, topic)
	respPull := httpGet(t, pullURL, tokenOutsider)
	respPull.Body.Close()
	if respPull.StatusCode != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for outsider pull, got %d", respPull.StatusCode)
	}

	// The group detail endpoint enforces the same boundary (kratos 403).
	respGroup := httpGet(t, testBaseURL+"/v1/groups/"+groupID, tokenOutsider)
	respGroup.Body.Close()
	if respGroup.StatusCode != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for outsider group detail, got %d", respGroup.StatusCode)
	}

	// Positive control: owner can read group details.
	respOwner := httpGet(t, testBaseURL+"/v1/groups/"+groupID, tokenOwner)
	respOwner.Body.Close()
	if respOwner.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 for owner group detail, got %d", respOwner.StatusCode)
	}

	t.Logf("group authorization enforced via openfga: group=%s topic=%s", groupID, topic)
}

// TestAuthz_FileDownloadPermissions verifies file authorization against
// OpenFGA: the owner downloads with 200, anonymous gets 401, and another
// authenticated user gets 403.
func TestAuthz_FileDownloadPermissions(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, userOwner := registerAndLogin(t, "authz-file-owner", "123456")
	tokenOther, userOther := registerAndLogin(t, "authz-file-other", "123456")

	content := []byte("authz secret attachment")
	resp := uploadFileMultipart(t, tokenOwner, "secret.txt", "text/plain", content)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload failed: status %d: %s", resp.StatusCode, body)
	}
	var reply v1.UploadFileReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode upload reply failed: %v", err)
	}
	downloadURL := testBaseURL + reply.Url

	// Owner downloads successfully.
	ownerReq, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("create owner download request failed: %v", err)
	}
	ownerReq.Header.Set("Authorization", "Bearer "+tokenOwner)
	ownerResp, err := http.DefaultClient.Do(ownerReq)
	if err != nil {
		t.Fatalf("owner download failed: %v", err)
	}
	defer ownerResp.Body.Close()
	if ownerResp.StatusCode != http.StatusOK {
		t.Fatalf("expected owner download 200, got %d", ownerResp.StatusCode)
	}

	// Anonymous download is rejected before authorization runs.
	anonResp, err := http.Get(downloadURL)
	if err != nil {
		t.Fatalf("anonymous download failed: %v", err)
	}
	anonResp.Body.Close()
	if anonResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected anonymous download 401, got %d", anonResp.StatusCode)
	}

	// Another authenticated user is forbidden by the OpenFGA file owner tuple.
	otherReq, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("create other download request failed: %v", err)
	}
	otherReq.Header.Set("Authorization", "Bearer "+tokenOther)
	otherResp, err := http.DefaultClient.Do(otherReq)
	if err != nil {
		t.Fatalf("other user download failed: %v", err)
	}
	defer otherResp.Body.Close()
	if otherResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected other user download 403, got %d", otherResp.StatusCode)
	}
	if userOwner == userOther {
		t.Fatalf("test setup error: owner and other user share id %d", userOwner)
	}

	t.Logf("file download permissions verified: file_id=%s", reply.FileId)
}

// TestAuthz_AgentShareLifecycle verifies agent sharing through the HTTP
// management API: the owner creates an agent, shares it with another real
// user who then sees it in listings and detail views, and loses access after
// the share is revoked.
func TestAuthz_AgentShareLifecycle(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	tokenOwner, _ := registerAndLogin(t, "authz-agent-owner", "123456")
	tokenB, userB := registerAndLogin(t, "authz-agent-userb", "123456")

	// Owner creates an agent and becomes its OpenFGA owner.
	createResp := httpPostAuth(t, testBaseURL+"/v1/agents", tokenOwner, map[string]any{
		"name":           "authz-agent",
		"instruction":    "you are a test agent",
		"modelServiceId": testModelServiceID,
		"modelId":        testModelID,
	})
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create agent failed: status %d: %s", createResp.StatusCode, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil || created.ID == 0 {
		t.Fatalf("decode created agent failed: %v (id=%d)", err, created.ID)
	}
	agentID := created.ID

	// Before sharing, B cannot see the agent.
	if listContainsAgent(t, tokenB, agentID) {
		t.Fatalf("expected agent %d to be invisible to user B before sharing", agentID)
	}

	// Owner shares the agent with user B.
	shareURL := fmt.Sprintf("%s/v1/agents/%d/shares", testBaseURL, agentID)
	shareResp := httpPutAuth(t, shareURL, tokenOwner, map[string]any{
		"kind":    "user",
		"target":  strconv.FormatInt(userB, 10),
		"enabled": true,
	})
	shareResp.Body.Close()
	if shareResp.StatusCode != http.StatusOK {
		t.Fatalf("share agent failed: status %d", shareResp.StatusCode)
	}

	// After sharing, B sees the agent in the list and can read its detail.
	if !listContainsAgent(t, tokenB, agentID) {
		t.Fatalf("expected agent %d to be visible to user B after sharing", agentID)
	}
	detailResp := httpGet(t, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenB)
	detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("expected detail 200 for shared user B, got %d", detailResp.StatusCode)
	}

	// Owner revokes the share.
	revokeResp := httpPutAuth(t, shareURL, tokenOwner, map[string]any{
		"kind":    "user",
		"target":  strconv.FormatInt(userB, 10),
		"enabled": false,
	})
	revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke agent share failed: status %d", revokeResp.StatusCode)
	}

	// After revocation the agent is invisible and detail access is denied.
	if listContainsAgent(t, tokenB, agentID) {
		t.Fatalf("expected agent %d to be invisible to user B after revocation", agentID)
	}
	deniedResp := httpGet(t, fmt.Sprintf("%s/v1/agents/%d", testBaseURL, agentID), tokenB)
	deniedResp.Body.Close()
	if deniedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected detail 403 for user B after revocation, got %d", deniedResp.StatusCode)
	}

	t.Logf("agent share lifecycle verified: agent=%d shared_with=%d", agentID, userB)
}

// listContainsAgent reports whether the user's agent listing contains the
// given agent ID.
func listContainsAgent(t *testing.T, token string, agentID int64) bool {
	t.Helper()
	resp := httpGet(t, testBaseURL+"/v1/agents", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list agents failed: status %d", resp.StatusCode)
	}
	var listing struct {
		Agents []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		t.Fatalf("decode agent listing failed: %v", err)
	}
	for _, a := range listing.Agents {
		if a.ID == agentID {
			return true
		}
	}
	return false
}
