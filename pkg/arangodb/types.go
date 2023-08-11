package arangodb

import (
	"github.com/sbezverk/gobmp/pkg/base"
)

type duplicateNode struct {
	Key         string       `json:"_key,omitempty"`
	DomainID    int64        `json:"domain_id"`
	IGPRouterID string       `json:"igp_router_id,omitempty"`
	AreaID      string       `json:"area_id"`
	Protocol    string       `json:"protocol,omitempty"`
	ProtocolID  base.ProtoID `json:"protocol_id,omitempty"`
	Name        string       `json:"name,omitempty"`
}
