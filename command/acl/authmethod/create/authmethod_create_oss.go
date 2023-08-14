// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !consulent
// +build !consulent

package authmethodcreate

import "github.com/hernad/consul/api"

type enterpriseCmd struct {
}

func (c *cmd) initEnterpriseFlags() {}

func (c *cmd) enterprisePopulateAuthMethod(method *api.ACLAuthMethod) error {
	return nil
}
