package events

import "encoding/json"

// RouteWorkEventPayload is the typed payload for formula-backed route and
// sling evidence events. Event type names carry the stage; the payload carries
// the concrete bead, target, formula, and failure context needed for live soak
// assertions without parsing CLI output.
type RouteWorkEventPayload struct {
	BeadID          string `json:"bead_id,omitempty"`
	Target          string `json:"target,omitempty"`
	RequestedTarget string `json:"requested_target,omitempty"`
	ClaimStoreRef   string `json:"claim_store_ref,omitempty"`
	Formula         string `json:"formula,omitempty"`
	Method          string `json:"method,omitempty"`
	WispRootID      string `json:"wisp_root_id,omitempty"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	StoreRef        string `json:"store_ref,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
	Idempotent      bool   `json:"idempotent,omitempty"`
}

// IsEventPayload marks RouteWorkEventPayload as an events.Payload variant.
func (RouteWorkEventPayload) IsEventPayload() {}

// RouteWorkPayloadJSON builds the JSON wire form for route/sling events.
func RouteWorkPayloadJSON(payload RouteWorkEventPayload) json.RawMessage {
	b, _ := json.Marshal(payload)
	return b
}

func init() {
	events := []string{
		RouteCreateSourceCreated,
		RouteCreateFormulaAttached,
		RouteCreateRouted,
		RouteCreateValidationFailed,
		SlingRouted,
		SlingFormulaAttached,
		SlingFormulaAttachmentSkipped,
		SlingFormulaAttachmentRejected,
	}
	for _, eventType := range events {
		RegisterPayload(eventType, RouteWorkEventPayload{})
	}
}
