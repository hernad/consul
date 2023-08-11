// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package types

import (
	"github.com/hashicorp/consul/internal/resource"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v1alpha1"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
)

type (
	DecodedHTTPRoute         = resource.DecodedResource[pbmesh.HTTPRoute, *pbmesh.HTTPRoute]
	DecodedGRPCRoute         = resource.DecodedResource[pbmesh.GRPCRoute, *pbmesh.GRPCRoute]
	DecodedTCPRoute          = resource.DecodedResource[pbmesh.TCPRoute, *pbmesh.TCPRoute]
	DecodedDestinationPolicy = resource.DecodedResource[pbmesh.DestinationPolicy, *pbmesh.DestinationPolicy]
	DecodedComputedRoutes    = resource.DecodedResource[pbmesh.ComputedRoutes, *pbmesh.ComputedRoutes]
	DecodedFailoverPolicy    = resource.DecodedResource[pbcatalog.FailoverPolicy, *pbcatalog.FailoverPolicy]
	DecodedService           = resource.DecodedResource[pbcatalog.Service, *pbcatalog.Service]
)
