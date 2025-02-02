// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !consulent
// +build !consulent

package instances

import (
	"github.com/hernad/consul/acl"
	"github.com/hernad/consul/api"
)

const showPartitionNamespace = false

func getBillableInstanceCounts(usage api.ServiceUsage, datacenter string) []serviceCount {
	return []serviceCount{
		{
			datacenter:    datacenter,
			partition:     acl.DefaultPartitionName,
			namespace:     acl.DefaultNamespaceName,
			instanceCount: usage.BillableServiceInstances,
			services:      usage.Services,
		},
	}
}

func getConnectInstanceCounts(usage api.ServiceUsage, datacenter string) []serviceCount {
	var counts []serviceCount

	for serviceType, instanceCount := range usage.ConnectServiceInstances {
		counts = append(counts, serviceCount{
			datacenter:    datacenter,
			partition:     acl.DefaultPartitionName,
			namespace:     acl.DefaultNamespaceName,
			serviceType:   serviceType,
			instanceCount: instanceCount,
		})
	}

	return counts
}
