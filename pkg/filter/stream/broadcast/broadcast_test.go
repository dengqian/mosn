package broadcast

import (
	"context"
	"github.com/valyala/fasthttp"
	v2 "mosn.io/mosn/pkg/config/v2"
	"mosn.io/mosn/pkg/upstream/cluster"
	"mosn.io/pkg/buffer"
	"net"
	"net/http"
	"testing"
	"time"

	"mosn.io/api"
	mosnctx "mosn.io/mosn/pkg/context"
	mosnhttp "mosn.io/mosn/pkg/protocol/http"
	_ "mosn.io/mosn/pkg/stream/http"
	"mosn.io/mosn/pkg/types"
)

type mockReceiveHandler struct {
	api.StreamReceiverFilterHandler
}

func (r *mockReceiveHandler) SendDirectResponse(headers api.HeaderMap, buf buffer.IoBuffer, trailers api.HeaderMap) {
	return
}

func (f *mockReceiveHandler) Route() api.Route {
	return &mockRoute{}
}

type mockRoute struct {
	api.Route
}

func (r *mockRoute) RouteRule() api.RouteRule {
	return &mockRouteRule{}
}

type mockRouteRule struct {
	api.RouteRule
	conf map[string]interface{}
}

func (r *mockRouteRule) PerFilterConfig() map[string]interface{} {
	perFilterConfig := map[string]interface{}{
		"broadcast": map[string]interface{}{
			"cluster": "test1",
		},
	}
	return perFilterConfig
}

func (r *mockRouteRule) UpstreamProtocol() string {
	return ""
}

type mockSendHandler struct {
	api.StreamSenderFilterHandler
}

func Test_BroadCastFilter(t *testing.T) {
	bc := BroadCastFilter{}
	receiveHandler := &mockReceiveHandler{}
	sendHandler := &mockSendHandler{}

	bc.upstreamProto = "Http1"
	bc.downstreamProto = "Http1"
	bc.SetReceiveFilterHandler(receiveHandler)
	bc.SetSenderFilterHandler(sendHandler)

	// reqHeaders := newHeaderMap(map[string]string{
	// 	protocol.MosnHeaderPathKey: "/br",
	// 	protocol.MosnHeaderHostKey: "br",
	// })

	reqHeaders := mosnhttp.RequestHeader{&fasthttp.RequestHeader{}, nil}
	reqHeaders.Add("test-multiple", "value-one")
	reqHeaders.Add("test-multiple", "value-two")

	// reqHeaders := mosnhttp.RequestHeader(map[string]string{
	// 	protocol.MosnHeaderPathKey: "/br",
	// 	protocol.MosnHeaderHostKey: "br",
	// })

	ctx := mosnctx.WithValue(context.Background(), types.ContextKeyDownStreamHeaders, reqHeaders)
	ctx = mosnctx.WithValue(ctx, types.ContextKeyConfigUpStreamProtocol, "Http1")
	ctx = mosnctx.WithValue(ctx, types.ContextKeyConfigDownStreamProtocol, "Http1")

	clusterConfig := v2.Cluster{
		Name:   "test1",
		LbType: v2.LB_RANDOM,
	}
	addr1 := "127.0.0.1:10000"
	addr2 := "127.0.0.1:10001"

	startServer(addr1)
	startServer(addr2)
	host1 := v2.Host{
		HostConfig: v2.HostConfig{
			Address: addr1,
		},
	}
	host2 := v2.Host{
		HostConfig: v2.HostConfig{
			Address: addr2,
		},
	}
	cluster.NewClusterManagerSingleton([]v2.Cluster{clusterConfig}, map[string][]v2.Host{
		"test1": {host1, host2},
	}, nil)
	bc.OnReceive(ctx, reqHeaders, nil, nil)
	time.Sleep(1 * time.Second)
	if couter[addr1] != 1 || couter[addr2] != 1 {
		t.Errorf("broadcast failed")
	}
}

var couter = make(map[string]int)

func handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("success"))
	couter[r.Host] = couter[r.Host] + 1
}

func startServer(addr string) {
	mux1 := http.NewServeMux()
	mux1.HandleFunc("/", handler)
	s := &http.Server{
		Addr:    addr,
		Handler: mux1,
	}
	ln, _ := net.Listen("tcp", s.Addr)
	couter[addr] = 0
	go s.Serve(ln)
}
