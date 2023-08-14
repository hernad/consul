// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cachetype

import (
	"context"
	"fmt"

	"github.com/hernad/consul/agent/cache"
	"github.com/hernad/consul/agent/structs"
)

// Recommended name for registration.
const NodeServicesName = "node-services"

// NodeServices supports fetching discovering service instances via the
// catalog.
type NodeServices struct {
	RegisterOptionsBlockingRefresh
	RPC RPC
}

func (c *NodeServices) Fetch(opts cache.FetchOptions, req cache.Request) (cache.FetchResult, error) {
	var result cache.FetchResult

	// The request should be a DCSpecificRequest.
	reqReal, ok := req.(*structs.NodeSpecificRequest)
	if !ok {
		return result, fmt.Errorf(
			"Internal cache failure: request wrong type: %T", req)
	}

	// Lightweight copy this object so that manipulating QueryOptions doesn't race.
	dup := *reqReal
	reqReal = &dup

	// Set the minimum query index to our current index so we block
	reqReal.QueryOptions.MinQueryIndex = opts.MinIndex
	reqReal.QueryOptions.MaxQueryTime = opts.Timeout

	// Always allow stale - there's no point in hitting leader if the request is
	// going to be served from cache and endup arbitrarily stale anyway. This
	// allows cached service-discover to automatically read scale across all
	// servers too.
	reqReal.AllowStale = true

	// Fetch
	var reply structs.IndexedNodeServices
	if err := c.RPC.RPC(context.Background(), "Catalog.NodeServices", reqReal, &reply); err != nil {
		return result, err
	}

	result.Value = &reply
	result.Index = reply.QueryMeta.Index
	return result, nil
}
