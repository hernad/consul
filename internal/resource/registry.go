// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type Registry interface {
	// Register the given resource type and its hooks.
	Register(reg Registration)

	// Resolve the given resource type and its hooks.
	Resolve(typ *pbresource.Type) (reg Registration, ok bool)
}

type Registration struct {
	// Type is the GVK of the resource type.
	Type *pbresource.Type

	// Proto is the resource's protobuf message type.
	Proto proto.Message

	// ACLs are hooks called to perform authorization on RPCs.
	ACLs *ACLHooks

	// Validate is called to structurally validate the resource (e.g.
	// check for required fields).
	Validate func(*pbresource.Resource) error

	// In the future, we'll add hooks, the controller etc. here.
	// TODO: https://github.com/hashicorp/consul/pull/16622#discussion_r1134515909
}

type ACLHooks struct {
	// Read is used to authorize Read RPCs and to filter results in List
	// RPCs.
	//
	// If it is omitted, `operator:read` permission is assumed.
	Read func(acl.Authorizer, *pbresource.ID) error

	// Write is used to authorize Write and Delete RPCs.
	//
	// If it is omitted, `operator:write` permission is assumed.
	Write func(acl.Authorizer, *pbresource.Resource) error

	// List is used to authorize List RPCs.
	//
	// If it is omitted, we only filter the results using Read.
	List func(acl.Authorizer, *pbresource.Tenancy) error
}

// Resource type registry
type TypeRegistry struct {
	// registrations keyed by GVK
	registrations map[string]Registration
	lock          sync.RWMutex
}

func NewRegistry() Registry {
	return &TypeRegistry{
		registrations: make(map[string]Registration),
	}
}

func (r *TypeRegistry) Register(registration Registration) {
	r.lock.Lock()
	defer r.lock.Unlock()

	typ := registration.Type
	if typ.Group == "" || typ.GroupVersion == "" || typ.Kind == "" {
		panic("type field(s) cannot be empty")
	}

	key := ToGVK(registration.Type)
	if _, ok := r.registrations[key]; ok {
		panic(fmt.Sprintf("resource type %s already registered", key))
	}

	// set default acl hooks for those not provided
	if registration.ACLs == nil {
		registration.ACLs = &ACLHooks{}
	}
	if registration.ACLs.Read == nil {
		registration.ACLs.Read = func(authz acl.Authorizer, id *pbresource.ID) error {
			return authz.ToAllowAuthorizer().OperatorReadAllowed(&acl.AuthorizerContext{})
		}
	}
	if registration.ACLs.Write == nil {
		registration.ACLs.Write = func(authz acl.Authorizer, resource *pbresource.Resource) error {
			return authz.ToAllowAuthorizer().OperatorWriteAllowed(&acl.AuthorizerContext{})
		}
	}
	if registration.ACLs.List == nil {
		registration.ACLs.List = func(authz acl.Authorizer, tenancy *pbresource.Tenancy) error {
			return authz.ToAllowAuthorizer().OperatorReadAllowed(&acl.AuthorizerContext{})
		}
	}

	// default validation to a no-op
	if registration.Validate == nil {
		registration.Validate = func(resource *pbresource.Resource) error { return nil }
	}

	r.registrations[key] = registration
}

func (r *TypeRegistry) Resolve(typ *pbresource.Type) (reg Registration, ok bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if registration, ok := r.registrations[ToGVK(typ)]; ok {
		return registration, true
	}
	return Registration{}, false
}

func ToGVK(resourceType *pbresource.Type) string {
	return fmt.Sprintf("%s.%s.%s", resourceType.Group, resourceType.GroupVersion, resourceType.Kind)
}