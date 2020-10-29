package broadcast

import (
	"context"

	"mosn.io/api"
	v2 "mosn.io/mosn/pkg/config/v2"
)

func init() {
	api.RegisterStream(v2.BroadCast, NewBroadCastConfig)
}

type BroadCastFactory struct {}

func NewBroadCastConfig(conf map[string]interface{}) (api.StreamFilterChainFactory, error) {
	return &BroadCastFactory{}, nil
}

func (bc *BroadCastFactory) CreateFilterChain(ctx context.Context, callbacks api.StreamFilterChainFactoryCallbacks) {
	bcf := &BroadCastFilter{}
	callbacks.AddStreamReceiverFilter(bcf, api.AfterRoute)
}
