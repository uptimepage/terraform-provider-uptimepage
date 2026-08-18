package client

import (
	"context"
	"fmt"
	"net/http"
)

// HeartbeatInfo is what GET /targets/{id}/heartbeat returns. Only PingURL is
// carried: the rest of the response is run telemetry (last ping, observed
// cadence, advice) that moves on its own and has no place in Terraform state.
//
// The endpoint sits behind the targets write scope even though it is a GET,
// because the ping URL is a capability: whoever holds it can report the job
// healthy or failed.
type HeartbeatInfo struct {
	// Null when the stored token cannot be decrypted, so treat it as optional.
	PingURL *string `json:"ping_url"`
}

// GetHeartbeat reads a heartbeat monitor's ping URL. The API answers 404 when
// the target is not a heartbeat, which surfaces as ErrNotFound.
func (c *Client) GetHeartbeat(ctx context.Context, id string) (*HeartbeatInfo, error) {
	var out HeartbeatInfo
	if err := c.do(ctx, http.MethodGet, targetsPath+"/"+id+"/heartbeat", nil, &out); err != nil {
		return nil, fmt.Errorf("get heartbeat %s: %w", id, err)
	}
	return &out, nil
}
