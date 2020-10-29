package broadcast

import (
	"context"
	"encoding/json"
	v2 "mosn.io/mosn/pkg/config/v2"
	mosnctx "mosn.io/mosn/pkg/context"
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/pkg/upstream/cluster"
	"net"

	"mosn.io/api"
	"mosn.io/pkg/buffer"
	//	"mosn.io/pkg/utils"
)

type BroadCastFilter struct {
	ctx             context.Context
	receiveHandler  api.StreamReceiverFilterHandler
	sendHandler     api.StreamSenderFilterHandler
	cluster         types.ClusterInfo
	upstreamProto   types.ProtocolName
	downstreamProto types.ProtocolName
	headers         api.HeaderMap
	upstreamHeaders api.HeaderMap
	sender          types.StreamSender
	host            types.Host
	conf            broadCastConfig
}

type broadCastConfig struct {
	Cluster string `json:"cluster,omitempty"`
}

// OnReceive is called with decoded request/response
func (bc *BroadCastFilter) OnReceive(ctx context.Context, headers api.HeaderMap, buf buffer.IoBuffer, trailers api.HeaderMap) api.StreamFilterStatus {
	// POST request not supported
	if buf != nil {
		return api.StreamFilterContinue
	}

	route := bc.receiveHandler.Route()
	if route == nil {
		log.DefaultLogger.Errorf("BroadCastFilter router not found")
		return api.StreamFilterContinue
	}
	rule := route.RouteRule()
	if rule == nil {
		log.DefaultLogger.Tracef("BroadCastFilter no route rule found")
		return api.StreamFilterContinue
	}

	bc.parsePerFilterConfig(rule.PerFilterConfig())
	if bc.conf.Cluster == "" {
		return api.StreamFilterContinue
	}

	clusterName := bc.conf.Cluster

	clusterManager := cluster.NewClusterManagerSingleton(nil, nil, nil)
	snap := clusterManager.GetClusterSnapshot(ctx, clusterName)
	if snap == nil {
		log.DefaultLogger.Errorf("BroadCastFilter cluster {%s} not found", clusterName)
		return api.StreamFilterStop
	}
	hostSet := snap.HostSet().Hosts()

	bc.ctx = mosnctx.WithValue(mosnctx.Clone(ctx), types.ContextKeyBufferPoolCtx, nil)
	bc.upstreamProto = bc.getUpstreamProtocol()
	bc.downstreamProto = bc.getUpstreamProtocol()
	connPool := clusterManager.ConnPoolForCluster(bc, snap, bc.getUpstreamProtocol())
	if connPool == nil {
		return api.StreamFilterStop
	}
	if headers != nil {
		// ! xprotocol should reimplement Clone function, not use default, trans protocol.CommonHeader
		h := headers.Clone()
		// nolint
		if _, ok := h.(protocol.CommonHeader); ok {
			log.DefaultLogger.Errorf("not support BroadCastFilter, protocal {%v} must implement Clone function",
				mosnctx.Get(bc.ctx, types.ContextKeyDownStreamProtocol))
			return api.StreamFilterStop
		}
		bc.headers = h
		bc.upstreamHeaders = bc.getUpstreamHeaders()
	}

	var (
		host         types.Host
		streamSender types.StreamSender
		failReason   types.PoolFailureReason
	)
	for i := 0; i < snap.HostNum(nil); i++ {
		connPool.UpdateHost(hostSet[i])
		host, streamSender, failReason = connPool.NewStream(bc.ctx, &receiver{})
		if failReason != "" {
			bc.OnFailure(failReason, host)
			continue
		}
		bc.OnReady(streamSender, host)
	}

	bc.receiveHandler.SendDirectResponse(headers, nil, nil)
	return api.StreamFilterStop
}

func (bc *BroadCastFilter) parsePerFilterConfig(perFilterConfig map[string]interface{}) {
	if perFilterConfig == nil {
		return
	}
	conf, exist := perFilterConfig[v2.BroadCast]
	if !exist {
		return
	}
	confStr, err := json.Marshal(conf)
	if err != nil {
		return
	}
	cfg := broadCastConfig{}
	if err = json.Unmarshal(confStr, &cfg); err != nil {
		return
	}

	bc.conf.Cluster = cfg.Cluster
}

func (bc *BroadCastFilter) SetReceiveFilterHandler(handler api.StreamReceiverFilterHandler) {
	bc.receiveHandler = handler
}

func (bc *BroadCastFilter) OnDestroy() {}

func (bc *BroadCastFilter) SetSenderFilterHandler(handler api.StreamSenderFilterHandler) {
	bc.sendHandler = handler
}

func (bc *BroadCastFilter) Append(ctx context.Context, headers api.HeaderMap, buf buffer.IoBuffer, trailers api.HeaderMap) api.StreamFilterStatus {
	return api.StreamFilterStop
}

func (bc *BroadCastFilter) getUpstreamProtocol() (currentProtocol types.ProtocolName) {
	configProtocol, ok := mosnctx.Get(bc.ctx, types.ContextKeyConfigUpStreamProtocol).(string)
	if !ok {
		configProtocol = string(protocol.Xprotocol)
	}

	if bc.receiveHandler.Route() != nil && bc.receiveHandler.Route().RouteRule() != nil && bc.receiveHandler.Route().RouteRule().UpstreamProtocol() != "" {
		configProtocol = bc.receiveHandler.Route().RouteRule().UpstreamProtocol()
	}

	if configProtocol == string(protocol.Auto) {
		currentProtocol = bc.getDownStreamProtocol()
	} else {
		currentProtocol = types.ProtocolName(configProtocol)
	}
	return currentProtocol
}

func (bc *BroadCastFilter) getDownStreamProtocol() (prot types.ProtocolName) {
	if dp, ok := mosnctx.Get(bc.ctx, types.ContextKeyConfigDownStreamProtocol).(string); ok {
		return types.ProtocolName(dp)
	}
	return bc.receiveHandler.RequestInfo().Protocol()
}

func (bc *BroadCastFilter) getUpstreamHeaders() types.HeaderMap {
	upstreamHeader, err := protocol.ConvertHeader(bc.ctx, bc.downstreamProto, bc.upstreamProto, bc.headers)
	if err != nil {
		log.Proxy.Warnf(bc.ctx, "[proxy] [upstream] [broadcast] convert header from %s to %s failed, %v",
			bc.downstreamProto, bc.upstreamProto, err)
		return bc.headers
	}
	return upstreamHeader
}

func (bc *BroadCastFilter) MetadataMatchCriteria() api.MetadataMatchCriteria {
	return nil
}

func (bc *BroadCastFilter) DownstreamConnection() net.Conn {
	return bc.receiveHandler.Connection().RawConn()
}

func (bc *BroadCastFilter) DownstreamHeaders() types.HeaderMap {
	return bc.headers
}

func (bc *BroadCastFilter) DownstreamContext() context.Context {
	return bc.ctx
}

func (bc *BroadCastFilter) DownstreamCluster() types.ClusterInfo {
	return bc.cluster
}

func (bc *BroadCastFilter) DownstreamRoute() api.Route {
	return bc.receiveHandler.Route()
}

func (bc *BroadCastFilter) OnFailure(reason types.PoolFailureReason, host types.Host) {}

func (bc *BroadCastFilter) OnReady(sender types.StreamSender, host types.Host) {
	sender.AppendHeaders(bc.ctx, bc.upstreamHeaders, true)
}
