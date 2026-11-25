package eino

import (
	"testing"

	"nonoka-im/internal/agent/port"
)

func TestUserMessageBuildsImagePart(t *testing.T) {
	msg := userMessage("look", []port.Attachment{{
		Kind: "image", Name: "pic.png", Mime: "image/png", Data: []byte("png"),
	}})
	if msg == nil || len(msg.UserInputMultiContent) != 2 {
		t.Fatalf("expected text+image parts, got %+v", msg)
	}
	if msg.UserInputMultiContent[0].Text != "look" {
		t.Fatalf("text part: %+v", msg.UserInputMultiContent[0])
	}
	image := msg.UserInputMultiContent[1].Image
	if image == nil || image.Base64Data == nil || image.MIMEType != "image/png" {
		t.Fatalf("image part: %+v", msg.UserInputMultiContent[1])
	}
}
