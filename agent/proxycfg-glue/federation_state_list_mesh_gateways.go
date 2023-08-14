// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxycfgglue

import (
	"context"

	"github.com/hashicorp/go-memdb"

	"github.com/hernad/consul/agent/cache"
	cachetype "github.com/hernad/consul/agent/cache-types"
	"github.com/hernad/consul/agent/consul/watch"
	"github.com/hernad/consul/agent/proxycfg"
	"github.com/hernad/consul/agent/structs"
	"github.com/hernad/consul/agent/structs/aclfilter"
)

// CacheFederationStateListMeshGateways satisfies the proxycfg.FederationStateListMeshGateways
// interface by sourcing data from the agent cache.
func CacheFederationStateListMeshGateways(c *cache.Cache) proxycfg.FederationStateListMeshGateways {
	return &cacheProxyDataSource[*structs.DCSpecificRequest]{c, cachetype.FederationStateListMeshGatewaysName}
}

// ServerFederationStateListMeshGateways satisfies the proxycfg.FederationStateListMeshGateways
// interface by sourcing data from a blocking query against the server's state
// store.
func ServerFederationStateListMeshGateways(deps ServerDataSourceDeps) proxycfg.FederationStateListMeshGateways {
	return &serverFederationStateListMeshGateways{deps}
}

type serverFederationStateListMeshGateways struct {
	deps ServerDataSourceDeps
}

func (s *serverFederationStateListMeshGateways) Notify(ctx context.Context, req *structs.DCSpecificRequest, correlationID string, ch chan<- proxycfg.UpdateEvent) error {
	return watch.ServerLocalNotify(ctx, correlationID, s.deps.GetStore,
		func(ws memdb.WatchSet, store Store) (uint64, *structs.DatacenterIndexedCheckServiceNodes, error) {
			authz, err := s.deps.ACLResolver.ResolveTokenAndDefaultMeta(req.Token, &req.EnterpriseMeta, nil)
			if err != nil {
				return 0, nil, err
			}

			index, fedStates, err := store.FederationStateList(ws)
			if err != nil {
				return 0, nil, err
			}

			results := make(map[string]structs.CheckServiceNodes)
			for _, fs := range fedStates {
				if gws := fs.MeshGateways; len(gws) != 0 {
					// Shallow clone to prevent ACL filtering manipulating the slice in memdb.
					results[fs.Datacenter] = gws.ShallowClone()
				}
			}

			rsp := &structs.DatacenterIndexedCheckServiceNodes{
				DatacenterNodes: results,
				QueryMeta: structs.QueryMeta{
					Index:   index,
					Backend: structs.QueryBackendBlocking,
				},
			}
			aclfilter.New(authz, s.deps.Logger).Filter(rsp)

			return index, rsp, nil
		},
		dispatchBlockingQueryUpdate[*structs.DatacenterIndexedCheckServiceNodes](ch),
	)
}
