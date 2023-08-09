// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package types

import (
	"google.golang.org/protobuf/proto"

	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type XRouteData interface {
	proto.Message
	XRouteWithRefs
}

type XRouteWithRefs interface {
	GetParentRefs() []*pbmesh.ParentReference
	GetUnderlyingBackendRefs() []*pbmesh.BackendReference
}

type DecodedXRoute struct {
	Resource *pbresource.Resource
	HTTP     *pbmesh.HTTPRoute
	GRPC     *pbmesh.GRPCRoute
	TCP      *pbmesh.TCPRoute
}

var _ XRouteWithRefs = (*DecodedXRoute)(nil)

func (d *DecodedXRoute) ToDecodedHTTPRoute() *DecodedHTTPRoute {
	return &DecodedHTTPRoute{Resource: d.Resource, Data: d.HTTP}
}

func (d *DecodedXRoute) ToDecodedGRPCRoute() *DecodedGRPCRoute {
	return &DecodedGRPCRoute{Resource: d.Resource, Data: d.GRPC}
}

func (d *DecodedXRoute) ToDecodedTCPRoute() *DecodedTCPRoute {
	return &DecodedTCPRoute{Resource: d.Resource, Data: d.TCP}
}

func (d *DecodedXRoute) GetParentRefs() []*pbmesh.ParentReference {
	if d == nil {
		return nil
	}
	switch {
	case d.HTTP != nil:
		return d.HTTP.GetParentRefs()
	case d.GRPC != nil:
		return d.GRPC.GetParentRefs()
	case d.TCP != nil:
		return d.TCP.GetParentRefs()
	default:
		return nil
	}
}

func (d *DecodedXRoute) GetUnderlyingBackendRefs() []*pbmesh.BackendReference {
	if d == nil {
		return nil
	}
	switch {
	case d.HTTP != nil:
		return d.HTTP.GetUnderlyingBackendRefs()
	case d.GRPC != nil:
		return d.GRPC.GetUnderlyingBackendRefs()
	case d.TCP != nil:
		return d.TCP.GetUnderlyingBackendRefs()
	default:
		return nil
	}
}
