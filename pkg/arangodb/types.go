package arangodb

import (
	"github.com/sbezverk/gobmp/pkg/base"
	"github.com/sbezverk/gobmp/pkg/sr"
	"github.com/sbezverk/gobmp/pkg/srv6"
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

type lsTopologyObject struct {
	Key                   string                `json:"_key"`
	From                  string                `json:"_from"`
	To                    string                `json:"_to"`
	Link                  string                `json:"link"`
	ProtocolID            base.ProtoID          `json:"protocol_id"`
	DomainID              int64                 `json:"domain_id"`
	MTID                  uint16                `json:"mt_id"`
	AreaID                string                `json:"area_id"`
	Protocol              string                `json:"protocol"`
	LocalLinkID           uint32                `json:"local_link_id"`
	RemoteLinkID          uint32                `json:"remote_link_id"`
	LocalLinkIP           string                `json:"local_link_ip"`
	RemoteLinkIP          string                `json:"remote_link_ip"`
	LocalNodeASN          uint32                `json:"local_node_asn"`
	RemoteNodeASN         uint32                `json:"remote_node_asn"`
	PeerNodeSID           *sr.PeerSID           `json:"peer_node_sid,omitempty"`
	PeerAdjSID            *sr.PeerSID           `json:"peer_adj_sid,omitempty"`
	PeerSetSID            *sr.PeerSID           `json:"peer_set_sid,omitempty"`
	SRv6BGPPeerNodeSID    *srv6.BGPPeerNodeSID  `json:"srv6_bgp_peer_node_sid,omitempty"`
	SRv6ENDXSID           []*srv6.EndXSIDTLV    `json:"srv6_endx_sid,omitempty"`
	LSAdjacencySID        []*sr.AdjacencySIDTLV `json:"ls_adjacency_sid,omitempty"`
	UnidirLinkDelay       uint32                `json:"unidir_link_delay"`
	UnidirLinkDelayMinMax []uint32              `json:"unidir_link_delay_min_max"`
	UnidirDelayVariation  uint32                `json:"unidir_delay_variation,omitempty"`
	UnidirPacketLoss      uint32                `json:"unidir_packet_loss,omitempty"`
	UnidirResidualBW      uint32                `json:"unidir_residual_bw,omitempty"`
	UnidirAvailableBW     uint32                `json:"unidir_available_bw,omitempty"`
	UnidirBWUtilization   uint32                `json:"unidir_bw_utilization,omitempty"`
}
