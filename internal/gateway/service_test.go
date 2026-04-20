package gateway

import (
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestBuildLoggedEventFromWebSocketEvent(t *testing.T) {
	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{
			Schema: "2.0",
			Header: &larkevent.EventHeader{
				EventID:   "evt_123",
				EventType: "im.message.receive_v1",
				TenantKey: "tenant_123",
				AppID:     "cli_test",
			},
		},
		EventReq: &larkevent.EventReq{
			Body: []byte(`{"schema":"2.0","header":{"event_id":"evt_123","event_type":"im.message.receive_v1"},"event":{"message":{"content":"{\"text\":\"hello gateway\"}"}}}`),
		},
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{
					OpenId: strPtr("ou_123"),
					UserId: strPtr("u_123"),
				},
				SenderType: strPtr("user"),
			},
			Message: &larkim.EventMessage{
				MessageId:   strPtr("om_123"),
				ChatId:      strPtr("oc_123"),
				ChatType:    strPtr("group"),
				MessageType: strPtr("text"),
				Content:     strPtr(`{"text":"hello gateway"}`),
			},
		},
	}

	entry, err := buildLoggedEvent(event)
	if err != nil {
		t.Fatalf("build logged event: %v", err)
	}

	if entry.EventType != "im.message.receive_v1" {
		t.Fatalf("unexpected event type: %s", entry.EventType)
	}
	if entry.MessageID != "om_123" {
		t.Fatalf("unexpected message_id: %s", entry.MessageID)
	}
	if entry.MessageText != "hello gateway" {
		t.Fatalf("unexpected message text: %s", entry.MessageText)
	}
	if entry.SenderOpenID != "ou_123" {
		t.Fatalf("unexpected sender open id: %s", entry.SenderOpenID)
	}
}

func strPtr(v string) *string {
	return &v
}
