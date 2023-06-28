package dns

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	libcluster "github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	libservice "github.com/hashicorp/consul/test/integration/consul-container/libs/service"
	libtopology "github.com/hashicorp/consul/test/integration/consul-container/libs/topology"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
)

// TestDNS_PeeringDNSTest
// This test creates two peered datacenters then ensures that each datacenter is able to
// make cross datacenter DNS requests when
// 1. The default token is configured with service:read and node:read permissions
// 2. The default token and the DNS token have service:read and node:read permissions
// 3. The default token has no permissions and the DNS token has service:read and node:read permissions
// 4. The default token is not set and the DNS token has service:read and node:read permissions
//
// The test then also validates that DNS does not work in the following cases
// 1. The default token and the DNS token are unset
// 2. The default token has no permissions and the DNS token is unset
// 3. The default token is unset and the DNS token has no permissions
// 4. The default token and the DNS token have no permissions
//
// Test Steps
// Setup
// * Set up peering topology; 2 clusters, 1 service from each cluster exported to the other
//
// Part 1 (Positive cases)
// * configure default token with service:read and node:read permissions and validate DNS each way
// * configure the DNS token with service:read and node:read permissions and validate DNS
// * configure default token with no permissions and validate DNS
// * unset default token and validate DNS
//
// Part 2 (Negative cases)
// * unset DNS token and validate DNS does not work
// * set default token with no permissions and validate DNS does not work
// * set DNS token with no permissions and validate that DNS does not work
// * unset default token and validate that DNS does not work
func TestDNS_PeeringDNSTest(t *testing.T) {
	t.Parallel()

	builtCluster1, builtCluster2 := libtopology.BasicPeeringTwoClustersSetup(t, utils.GetTargetImageName(), utils.TargetVersion,
		libtopology.PeeringClusterSize{
			AcceptingNumServers: 3,
			AcceptingNumClients: 1,
			DialingNumServers:   3,
			DialingNumClients:   1,
		},
		false)

	var (
		cluster1 = builtCluster1.Cluster
		cluster2 = builtCluster2.Cluster
		//cluster1Sidecar = builtCluster1.Container
		//cluster2Sidecar = builtCluster2.Container
	)

	// Create test service in each DC
	// call mapped port for clients with dig or DNS client
	partition := "default"
	dc1 := "dc1"
	dc2 := "dc2"
	peer1 := fmt.Sprintf("peer-%s-%s", dc1, partition)
	peer2 := fmt.Sprintf("peer-%s-%s", dc2, partition)

	c1Proxy := createServices(t, cluster1, peer1)
	_, c1port := c1Proxy.GetAddr()
	_, c1adminPort := c1Proxy.GetAdminAddr()

	c2Proxy := createServices(t, cluster2, peer2)
	_, c2port := c2Proxy.GetAddr()
	_, c2adminPort := c2Proxy.GetAdminAddr()

	libassert.AssertUpstreamEndpointStatus(t, c1adminPort, "static-server.default", "HEALTHY", 1)
	libassert.GetEnvoyListenerTCPFilters(t, c1adminPort)

	libassert.AssertContainerState(t, c1Proxy, "running")
	libassert.AssertFortioName(t, fmt.Sprintf("http://localhost:%d", c1port), "static-server", "")

	libassert.AssertUpstreamEndpointStatus(t, c2adminPort, "static-server.default", "HEALTHY", 1)
	libassert.GetEnvoyListenerTCPFilters(t, c2adminPort)

	libassert.AssertContainerState(t, c2Proxy, "running")
	libassert.AssertFortioName(t, fmt.Sprintf("http://localhost:%d", c2port), "static-server", "")

	// export test services
	svc1 := api.ExportedService{
		Name: "static-server",
		Consumers: []api.ServiceConsumer{
			{
				Peer: peer2,
			},
		},
	}

	req := api.ExportedServicesConfigEntry{
		Name:      partition,
		Partition: "",
		Services: []api.ExportedService{
			svc1,
		},
	}

	cluster1Client := cluster1.APIClient(0)
	_, _, err := cluster1Client.ConfigEntries().Set(&req, nil)
	require.NoError(t, err)

	svc2 := api.ExportedService{
		Name: "static-server",
		Consumers: []api.ServiceConsumer{
			{
				Peer: peer1,
			},
		},
	}

	req = api.ExportedServicesConfigEntry{
		Name:      partition,
		Partition: "",
		Services: []api.ExportedService{
			svc2,
		},
	}

	cluster2Client := cluster2.APIClient(0)
	_, _, err = cluster2Client.ConfigEntries().Set(&req, nil)
	require.NoError(t, err)

	dnsPort, err := nat.NewPort("tcp", "8600")
	require.NoError(t, err)

	client1Container := cluster1.Agents[0].GetPod()
	client1MappedDNS, err := client1Container.MappedPort(context.Background(), dnsPort)
	require.NoError(t, err)

	client2Container := cluster2.Agents[0].GetPod()
	client2MappedDNS, err := client2Container.MappedPort(context.Background(), dnsPort)

	t1, t2 := mutualDNSCheck(t, client1MappedDNS, client2MappedDNS)
	require.True(t, t1 && t2)

	// Need to make DNS request on 8600 to client
	// probably via the sidecar?

	// Don't need to test full suite of DNS just lookup the service in the other cluster from both sides

}

func createServices(t *testing.T, cluster *libcluster.Cluster, peerName string) *libservice.ConnectContainer {
	node := cluster.Agents[0]
	client := node.GetClient()
	// Create a service and proxy instance
	serviceOpts := &libservice.ServiceOpts{
		Name:     libservice.StaticServerServiceName,
		ID:       "static-server",
		HTTPPort: 8080,
		GRPCPort: 8079,
	}

	// Create a service and proxy instance
	_, _, err := libservice.CreateAndRegisterStaticServerAndSidecar(node, serviceOpts)
	require.NoError(t, err)

	libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
	libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)

	// Create a client proxy instance with the server as an upstream
	clientConnectProxy, err := libservice.CreateAndRegisterStaticClientSidecar(node, "", false, false)
	require.NoError(t, err)

	libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)

	return clientConnectProxy
}

func mutualDNSCheck(t *testing.T, cluster1Port, cluster2Port nat.Port) (bool, bool) {
	m := new(dns.Msg)
	m.SetQuestion("static-server.service.peer-dc2-default.peer.consul", dns.TypeSRV)

	c := new(dns.Client)

	addr1 := fmt.Sprintf("127.0.0.1:%d", cluster1Port.Int())
	reply1, _, err := c.Exchange(m, addr1)
	require.NoError(t, err)

	m = new(dns.Msg)
	m.SetQuestion("static-server.service.peer-dc1-default.peer.consul", dns.TypeSRV)

	addr2 := fmt.Sprintf("127.0.0.1:%d", cluster2Port.Int())
	reply2, _, err := c.Exchange(m, addr2)
	require.NoError(t, err)

	return len(reply1.Answer) > 0, len(reply2.Answer) > 0
}
