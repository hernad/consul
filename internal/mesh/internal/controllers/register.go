// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package controllers

import (
	"github.com/hashicorp/consul/internal/controller"
	"github.com/hashicorp/consul/internal/mesh/internal/controllers/routes"
)

type Dependencies struct {
}

func Register(mgr *controller.Manager, deps Dependencies) {
	mgr.Register(routes.Controller())
}
