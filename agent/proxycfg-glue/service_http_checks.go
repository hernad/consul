// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxycfgglue

import (
	"context"

	"github.com/hernad/consul/agent/cache"
	cachetype "github.com/hernad/consul/agent/cache-types"
	"github.com/hernad/consul/agent/local"
	"github.com/hernad/consul/agent/proxycfg"
	"github.com/hernad/consul/agent/structs"
)

// CacheHTTPChecks satisifies the proxycfg.HTTPChecks interface by sourcing
// data from the agent cache.
func CacheHTTPChecks(c *cache.Cache) proxycfg.HTTPChecks {
	return &cacheProxyDataSource[*cachetype.ServiceHTTPChecksRequest]{c, cachetype.ServiceHTTPChecksName}
}

// ServerHTTPChecks satisifies the proxycfg.HTTPChecks interface.
// It sources data from the server agent cache if the service exists in the local state, else
// it is a no-op since the checks can only be performed by local agents.
func ServerHTTPChecks(deps ServerDataSourceDeps, nodeName string, cacheSource proxycfg.HTTPChecks, localState *local.State) proxycfg.HTTPChecks {
	return serverHTTPChecks{deps, nodeName, cacheSource, localState}
}

type serverHTTPChecks struct {
	deps        ServerDataSourceDeps
	nodeName    string
	cacheSource proxycfg.HTTPChecks
	localState  *local.State
}

func (c serverHTTPChecks) Notify(ctx context.Context, req *cachetype.ServiceHTTPChecksRequest, correlationID string, ch chan<- proxycfg.UpdateEvent) error {
	// if the service exists in the current server agent local state, delegate to the cache data source
	if req.NodeName == c.nodeName && c.localState.ServiceExists(structs.ServiceID{ID: req.ServiceID, EnterpriseMeta: req.EnterpriseMeta}) {
		return c.cacheSource.Notify(ctx, req, correlationID, ch)
	}
	l := c.deps.Logger.With("service_id", req.ServiceID)
	l.Debug("service-http-checks: no-op")
	return nil
}
