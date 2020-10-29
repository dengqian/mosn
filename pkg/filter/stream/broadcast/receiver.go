package broadcast

import (
	"context"

	"mosn.io/api"
	"mosn.io/pkg/buffer"
)

type receiver struct{
	bcf BroadCastFilter
}

func (r *receiver) OnReceive(ctx context.Context, headers api.HeaderMap, data buffer.IoBuffer, trailers api.HeaderMap) {
	// http1 close client
}

func (r *receiver) OnDecodeError(ctx context.Context, err error, headers api.HeaderMap) {

}

